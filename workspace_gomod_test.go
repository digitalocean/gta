package gta

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// testBaseReaderDiffer is a Differ that also implements BaseFileReader,
// simulating the git differ's ReadBaseFile. The real implementation runs
//
//	git -C <repo toplevel> show <baseRef>:<relativePath>
//
// (see (*git).readBaseFile in differ.go), so the relativePath it receives is
// always interpreted relative to the git repository root. baseFiles is
// therefore keyed by repo-root-relative paths. requested records every path
// the code under test asks for, so a failure can show *which* file was
// consulted, not just that detection failed.
type testBaseReaderDiffer struct {
	testDiffer
	baseFiles map[string][]byte
	requested []string
}

var _ BaseFileReader = &testBaseReaderDiffer{}

func (d *testBaseReaderDiffer) ReadBaseFile(relativePath string) ([]byte, error) {
	d.requested = append(d.requested, relativePath)
	b, ok := d.baseFiles[relativePath]
	if !ok {
		return nil, fmt.Errorf("path %q does not exist at base ref", relativePath)
	}
	return b, nil
}

// TestChangedPackages_WorkspaceGoModPreciseDiff demonstrates a false negative
// in the precise go.mod diff analysis when running in Go workspace mode.
//
// Scenario (fixture: testdata/workspacegomod):
//
//	<repo root>            <- git repository toplevel
//	├── go.work            uses ./modA and ./modB (root module NOT in workspace)
//	├── go.mod             root tooling module, requires example.test/dep v1.1.0
//	├── modA/              workspace module, no external deps
//	├── modB/              workspace module, requires example.test/dep
//	│   ├── go.mod         base ref: v1.0.0 -> HEAD: v1.1.0 (the only change)
//	│   └── b.go           imports example.test/dep
//	└── depsrc/            local source of example.test/dep (via go.work replace)
//
// The change between the base ref and HEAD is a single dependency bump in
// modB/go.mod: example.test/dep v1.0.0 -> v1.1.0. Because modB's package
// imports example.test/dep, GTA must mark workspace.gomod/modB as changed.
//
// Why the current code misses it:
//
//  1. In workspace mode, workspaceroots() sets g.roots to the workspace
//     module directories taken from the go.work use directives:
//     [<root>/modA, <root>/modB]. The repository root itself is not in
//     g.roots unless go.work happens to contain "use ." ordered before the
//     module that changed.
//
//  2. markedPackages() sees modB/go.mod in the diff and calls
//     relativeModFilePath(<root>/modB, g.roots, "go.mod") to compute the
//     path to pass to BaseFileReader.ReadBaseFile. The first root that
//     prefix-matches <root>/modB is <root>/modB itself, so the function
//     returns "go.mod" -- a path relative to the *module* directory.
//
//  3. ReadBaseFile resolves paths relative to the *git repository root*
//     (git show <base>:go.mod). Since the repo root contains its own go.mod
//     (the tooling module), the read SUCCEEDS but returns the wrong file:
//     the root go.mod instead of modB/go.mod.
//
//  4. diffGoMod then compares the base-ref ROOT go.mod against the new
//     modB/go.mod. diffGoMod only inspects require/replace directives, so
//     the differing module lines are invisible to it. The root module
//     already requires example.test/dep v1.1.0 -- the very version modB just
//     bumped to, a routine "catch up to the version the rest of the repo
//     uses" situation in a monorepo. The requires match, the diff comes back
//     empty, and preciseDetection is reported true with zero changed module
//     paths.
//
//  5. With preciseDetection == true and no changed module paths, neither the
//     precise branch nor the nuclear fallback marks anything. modB's diff
//     contains no .go files, so the directory is then skipped entirely.
//     Result: ChangedPackages reports nothing -- a false negative for a
//     dependency change that affects workspace.gomod/modB.
//
// The test asserts the CORRECT behavior (workspace.gomod/modB is reported),
// so it fails on the current code, demonstrating the bug. Note the exact
// equality assertion also rules out passing via the nuclear fallback, which
// would mark modA as well.
func TestChangedPackages_WorkspaceGoModPreciseDiff(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	wsDir, err := filepath.Abs(filepath.Join("testdata", "workspacegomod"))
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir(wsDir); err != nil {
		t.Fatal(err)
	}

	// The on-disk fixture represents the NEW state (modB requires
	// example.test/dep v1.1.0). The differ below serves the base-ref state:
	// modB/go.mod still at v1.0.0, and the root go.mod unchanged between the
	// base ref and HEAD.
	rootGoModAtBase, err := os.ReadFile(filepath.Join(wsDir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	modBGoModAtBase := []byte(`module workspace.gomod/modB

go 1.21

require example.test/dep v1.0.0
`)

	modBDir := filepath.Join(wsDir, "modB")
	difr := &testBaseReaderDiffer{
		testDiffer: testDiffer{
			diff: map[string]Directory{
				// The only change between base and HEAD: modB/go.mod.
				modBDir: {Exists: true, Files: []string{"go.mod"}},
			},
		},
		baseFiles: map[string][]byte{
			// Keyed by git-repo-root-relative path, as git show would see it.
			"go.mod":      rootGoModAtBase,
			"modB/go.mod": modBGoModAtBase,
		},
	}

	gta, err := New(SetDiffer(difr))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	pkgs, err := gta.ChangedPackages()
	if err != nil {
		t.Fatalf("ChangedPackages() error: %v", err)
	}

	var gotPaths []string
	for _, pkg := range pkgs.AllChanges {
		gotPaths = append(gotPaths, pkg.ImportPath)
	}

	// modB's package imports example.test/dep, whose required version
	// changed, so it -- and only it -- must be reported.
	wantPaths := []string{"workspace.gomod/modB"}

	if diff := cmp.Diff(wantPaths, gotPaths); diff != "" {
		t.Errorf("AllChanges import paths (-want +got):\n%s", diff)
		t.Logf("base files consulted via ReadBaseFile: %q", difr.requested)
		t.Logf("(a request for %q instead of %q means the root go.mod was diffed against modB's new go.mod)", "go.mod", "modB/go.mod")
	}
}
