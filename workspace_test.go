package gta

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestChangedPackages_SymlinkedRoot verifies that gta correctly resolves
// changed packages when the differ supplies a directory path that passes
// through a symlink, while packages.Load reports Module.Dir in resolved
// (canonical) form. This exercises canonicalDir in resolveLocal and the
// root-normalization in New().
func TestChangedPackages_SymlinkedRoot(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	// Resolve the real (canonical) path to the workspace fixture.
	wsDir, err := filepath.Abs(filepath.Join("testdata", "workspacetest"))
	if err != nil {
		t.Fatal(err)
	}
	realWsDir, err := filepath.EvalSymlinks(wsDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create a symlink pointing at the workspace fixture directory.
	linkDir := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(realWsDir, linkDir); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	// Change into the workspace via the symlinked path so that the module
	// root reported by `go list`/`go env` is the symlinked path.
	if err := os.Chdir(linkDir); err != nil {
		t.Fatal(err)
	}

	// The differ reports the changed directory through the symlink path
	// (as if the caller reached the repo via that symlink).
	bPkgDirViaLink := filepath.Join(linkDir, "modB", "pkg")

	difr := &testDiffer{
		diff: map[string]Directory{
			bPkgDirViaLink: {Exists: true, Files: []string{"b.go"}},
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
	sort.Strings(gotPaths)

	// Expect the same result as TestChangedPackages_WorkspaceCrossModule:
	// modB/pkg changed, and its dependents modA/pkg and modC/pkg should also
	// be detected. An empty result here means symlink paths failed to match.
	wantPaths := []string{
		"workspace.test/modA/pkg",
		"workspace.test/modB/pkg",
		"workspace.test/modC/pkg",
	}

	if diff := cmp.Diff(wantPaths, gotPaths); diff != "" {
		t.Errorf("AllChanges import paths (-want +got):\n%s\n(empty result indicates symlink path normalization is broken)", diff)
	}
}

// TestChangedPackages_SymlinkedTestdataRoot guards EXTRA-2: the
// `abs = canonicalDir(abs)` line in markedPackages() that canonicalizes the
// differ-supplied path BEFORE the isIgnoredByGo check.
//
// Without EXTRA-2, a differ path whose un-resolved form contains a "testdata"
// ancestor segment is silently dropped by isIgnoredByGo — even when its
// canonical (EvalSymlinks) form lives under a registered module root.
//
// Mechanism reproduced here:
//  1. The GTA roots are canonical: realWsDir = EvalSymlinks(testdata/workspacetest).
//  2. A symlink `link` in a tempdir points at the gta module root (parent of
//     testdata/), so `link/testdata/workspacetest/modB/pkg` is a valid path
//     whose un-resolved form contains the "testdata" segment.
//  3. The differ supplies `link/testdata/workspacetest/modB/pkg` as the changed
//     directory.
//  4. Without EXTRA-2: isIgnoredByGo walks up link/.../workspacetest (not in
//     roots), reaches "testdata" → returns true → package silently dropped.
//  5. With EXTRA-2: abs is canonicalized first → the roots match fires before
//     "testdata" is reached → package is detected normally.
func TestChangedPackages_SymlinkedTestdataRoot(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	// Resolve the canonical path to the workspace fixture dir (the GTA root).
	wsDir, err := filepath.Abs(filepath.Join("testdata", "workspacetest"))
	if err != nil {
		t.Fatal(err)
	}
	realWsDir, err := filepath.EvalSymlinks(wsDir)
	if err != nil {
		t.Fatal(err)
	}

	// Resolve the canonical path to the gta module root (parent of testdata/).
	gtaRoot, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	realGtaRoot, err := filepath.EvalSymlinks(gtaRoot)
	if err != nil {
		t.Fatal(err)
	}

	// Create a symlink in a tempdir pointing at the gta module root so that
	// link/testdata/workspacetest/modB/pkg is a valid path containing the
	// literal "testdata" segment but resolves canonically to the real modB/pkg.
	linkDir := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(realGtaRoot, linkDir); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	// Change into the workspace via the REAL (canonical) path so that
	// toplevel() returns the canonical root.  This ensures g.roots is
	// canonical — the precondition that makes isIgnoredByGo's testdata trap
	// fire when the un-canonicalized differ path is used.
	if err := os.Chdir(realWsDir); err != nil {
		t.Fatal(err)
	}

	// The differ supplies the changed directory through the symlinked path.
	// Its un-resolved form is:  link/testdata/workspacetest/modB/pkg
	// Its EvalSymlinks form is: <realGtaRoot>/testdata/workspacetest/modB/pkg
	bPkgDirViaLink := filepath.Join(linkDir, "testdata", "workspacetest", "modB", "pkg")

	difr := &testDiffer{
		diff: map[string]Directory{
			bPkgDirViaLink: {Exists: true, Files: []string{"b.go"}},
		},
	}

	// Use the canonical workspace root as the explicit root so that the roots
	// are always in resolved form regardless of the CWD.
	g, err := New(SetDiffer(difr), SetRoots(realWsDir))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	pkgs, err := g.ChangedPackages()
	if err != nil {
		t.Fatalf("ChangedPackages() error: %v", err)
	}

	var gotPaths []string
	for _, pkg := range pkgs.AllChanges {
		gotPaths = append(gotPaths, pkg.ImportPath)
	}
	sort.Strings(gotPaths)

	// Expect the same result as TestChangedPackages_WorkspaceCrossModule.
	// An empty result indicates that EXTRA-2 (abs = canonicalDir(abs)) is
	// missing and the package was silently dropped by the testdata guard.
	wantPaths := []string{
		"workspace.test/modA/pkg",
		"workspace.test/modB/pkg",
		"workspace.test/modC/pkg",
	}

	if diff := cmp.Diff(wantPaths, gotPaths); diff != "" {
		t.Errorf("AllChanges import paths (-want +got):\n%s\n(empty result means the symlinked differ path containing 'testdata' was silently dropped — EXTRA-2 is broken)", diff)
	}
}

func TestChangedPackages_WorkspaceCrossModule(t *testing.T) {
	// Test that changing a package in modB causes dependents in modA and
	// transitively modC to be detected.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	wsDir, err := filepath.Abs(filepath.Join("testdata", "workspacetest"))
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir(wsDir); err != nil {
		t.Fatal(err)
	}

	// Build the differ: modB/pkg/b.go changed.
	bPkgDir := filepath.Join(wsDir, "modB", "pkg")
	difr := &testDiffer{
		diff: map[string]Directory{
			bPkgDir: {Exists: true, Files: []string{"b.go"}},
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

	// Expect: workspace.test/modB/pkg changed, and its dependents
	// workspace.test/modA/pkg and workspace.test/modC/pkg should also be
	// marked.
	var gotPaths []string
	for _, pkg := range pkgs.AllChanges {
		gotPaths = append(gotPaths, pkg.ImportPath)
	}
	sort.Strings(gotPaths)

	wantPaths := []string{
		"workspace.test/modA/pkg",
		"workspace.test/modB/pkg",
		"workspace.test/modC/pkg",
	}

	if diff := cmp.Diff(wantPaths, gotPaths); diff != "" {
		t.Errorf("AllChanges import paths (-want +got):\n%s", diff)
	}

	// Verify the direct change.
	var changePaths []string
	for _, pkg := range pkgs.Changes {
		changePaths = append(changePaths, pkg.ImportPath)
	}
	if len(changePaths) != 1 || changePaths[0] != "workspace.test/modB/pkg" {
		t.Errorf("Changes = %v; want [workspace.test/modB/pkg]", changePaths)
	}
}

func TestChangedPackages_WorkspaceIsolatedModule(t *testing.T) {
	// Test that changing a package in a module with no dependents only
	// reports that one package.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	wsDir, err := filepath.Abs(filepath.Join("testdata", "workspacetest"))
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir(wsDir); err != nil {
		t.Fatal(err)
	}

	// modC/pkg has no dependents within the workspace.
	cPkgDir := filepath.Join(wsDir, "modC", "pkg")
	difr := &testDiffer{
		diff: map[string]Directory{
			cPkgDir: {Exists: true, Files: []string{"c.go"}},
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

	wantPaths := []string{"workspace.test/modC/pkg"}
	if diff := cmp.Diff(wantPaths, gotPaths); diff != "" {
		t.Errorf("AllChanges import paths (-want +got):\n%s", diff)
	}
}

func TestChangedPackages_GoWorkFileChanged(t *testing.T) {
	// When go.work itself is changed, all packages should be marked.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	wsDir, err := filepath.Abs(filepath.Join("testdata", "workspacetest"))
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir(wsDir); err != nil {
		t.Fatal(err)
	}

	// go.work changed.
	difr := &testDiffer{
		diff: map[string]Directory{
			wsDir: {Exists: true, Files: []string{"go.work"}},
		},
	}

	gta, err := New(SetDiffer(difr), SetPrefixes("workspace.test/"))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	pkgs, err := gta.ChangedPackages()
	if err != nil {
		t.Fatalf("ChangedPackages() error: %v", err)
	}

	// When go.work changes, all workspace packages should be in AllChanges.
	if len(pkgs.AllChanges) < 3 {
		var paths []string
		for _, p := range pkgs.AllChanges {
			paths = append(paths, p.ImportPath)
		}
		t.Errorf("expected at least 3 changed packages when go.work changes, got %d: %v", len(pkgs.AllChanges), paths)
	}
}
