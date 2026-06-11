package gta

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// testBaseReaderDiffer is a Differ that also implements BaseFileReader,
// simulating the git differ's ReadBaseFile. The real implementation receives
// an absolute path and resolves it against the git repository toplevel
// (see (*git).readBaseFile in differ.go). root plays the role of the git
// toplevel; baseFiles is keyed by repo-root-relative paths. requested records
// every absolute path the code under test passes in, so a failure can show
// *which* file was consulted, not just that detection failed.
type testBaseReaderDiffer struct {
	testDiffer
	root      string
	baseFiles map[string][]byte
	requested []string
}

var _ BaseFileReader = &testBaseReaderDiffer{}

func (d *testBaseReaderDiffer) ReadBaseFile(absPath string) ([]byte, error) {
	d.requested = append(d.requested, absPath)
	rel, err := filepath.Rel(d.root, absPath)
	if err != nil {
		return nil, err
	}
	b, ok := d.baseFiles[rel]
	if !ok {
		return nil, fmt.Errorf("path %q does not exist at base ref", rel)
	}
	return b, nil
}

// TestChangedPackages_WorkspaceGoModPreciseDiff is a regression test for the
// path-resolution contract: ReadBaseFile receives absolute paths and the differ
// resolves them against the repository root; under the fork's multi-root
// semantics this scenario produced a false negative (root go.mod silently
// diffed against modB's new go.mod).
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
// Historical 5-step failure mechanism (before path-resolution was moved into
// the differ):
//
//  1. In workspace mode, workspaceroots() set g.roots to the workspace module
//     directories taken from go.work use directives: [<root>/modA, <root>/modB].
//     The repository root itself was not in g.roots unless go.work happened to
//     contain "use ." ordered before the module that changed.
//
//  2. markedPackages() saw modB/go.mod in the diff and called
//     relativeModFilePath(<root>/modB, g.roots, "go.mod") to compute the path
//     to pass to BaseFileReader.ReadBaseFile. The first root that
//     prefix-matched <root>/modB was <root>/modB itself, so the function
//     returned "go.mod" -- a path relative to the *module* directory.
//
//  3. ReadBaseFile resolved paths relative to the *git repository root*
//     (git show <base>:go.mod). Since the repo root contained its own go.mod
//     (the tooling module), the read SUCCEEDED but returned the wrong file:
//     the root go.mod instead of modB/go.mod.
//
//  4. diffGoMod compared the base-ref ROOT go.mod against the new modB/go.mod.
//     diffGoMod only inspects require/replace directives, so the differing
//     module lines were invisible to it. The root module already required
//     example.test/dep v1.1.0 -- the very version modB just bumped to, a
//     routine "catch up to the version the rest of the repo uses" situation in
//     a monorepo. The requires matched, the diff came back empty, and
//     preciseDetection was reported true with zero changed module paths.
//
//  5. With preciseDetection == true and no changed module paths, neither the
//     precise branch nor the nuclear fallback marked anything. modB's diff
//     contained no .go files, so the directory was then skipped entirely.
//     Result: ChangedPackages reported nothing -- a false negative for a
//     dependency change that affected workspace.gomod/modB.
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
		root: wsDir,
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
