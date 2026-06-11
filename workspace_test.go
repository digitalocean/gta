package gta

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

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
