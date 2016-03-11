// Package gta provides a set of utilites to build a set of dirty packages and their dependents
// that can be used to target code changes.
package gta

import (
	"errors"
	"fmt"
	"go/build"
	"path/filepath"
	"sort"
)

var (
	// ErrNoDiffer is returned when there is no differ set on the GTA.
	ErrNoDiffer = errors.New("there is no differ set")
	// ErrNoPackager is returned when there is no packager set on the GTA.
	ErrNoPackager = errors.New("there is no packager set")
)

// A GTA provides a method of building dirty packages, and their dependent packages.
type GTA struct {
	differ   Differ
	packager Packager
}

// New returns a new GTA with various options passed to New.
func New(opts ...Option) (*GTA, error) {
	gta := &GTA{
		differ:   &Git{},
		packager: DefaultPackager,
	}

	for _, opt := range opts {
		err := opt(gta)
		if err != nil {
			return nil, err
		}
	}

	return gta, nil
}

// DirtyPackages uses the differ and packager to build a list of dirty packages where dirty is defined as "changed".
func (g *GTA) DirtyPackages() ([]*build.Package, error) {
	if g.differ == nil {
		return nil, ErrNoDiffer
	}
	if g.packager == nil {
		return nil, ErrNoPackager
	}

	// get our diff'd directories
	dirs, err := g.differ.Diff()
	if err != nil {
		return nil, fmt.Errorf("diffing directory for dirty packages, %v", err)
	}

	// we build our set of initial dirty packages from the git diff
	changed := make(map[string]bool)
	for dir := range dirs {
		// Avoid .foo, _foo, and testdata directory trees how the go tool does!
		// See https://github.com/golang/tools/blob/3a85b8d/go/buildutil/allpackages.go#L93
		// Above link is not guranteed to work.
		base := filepath.Base(dir)
		parent := filepath.Base(filepath.Dir(dir))
		if base == "" || base[0] == '.' || base[0] == '_' || base == "testdata" || parent == "testdata" {
			continue
		}
		pkg, err := g.packager.PackageFromDir(dir)
		if err != nil {
			if _, ok := err.(*build.NoGoError); ok {
				// there are no buildable go files in this directory
				// so no dirty packges
				continue
			}
			return nil, fmt.Errorf("pulling package information for %q, %v", dir, err)
		}
		// we create a simple set of changed pkgs by import path
		changed[pkg.ImportPath] = false
	}

	// we build the dependent graph
	graph, err := g.packager.DependentGraph()
	if err != nil {
		return nil, fmt.Errorf("building dependency graph, %v", err)
	}

	// we copy the map since iterating over a map
	// while its being mutated is undefined behavior
	marked := make(map[string]bool)
	for k, v := range changed {
		marked[k] = v
	}

	for change := range changed {
		// we traverse the graph and build our list of mark all dependents
		graph.Traverse(change, marked)
	}

	// build our packages
	var packages []*build.Package
	for path := range marked {
		pkg, err := g.packager.PackageFromImport(path)
		if err != nil {
			if _, ok := err.(*build.NoGoError); ok {
				// there are no buildable go files in this directory
				// so no dirty packges
				continue
			}
			return nil, fmt.Errorf("building packages for %q: %v", path, err)
		}
		packages = append(packages, pkg)
	}

	sort.Sort(byPackageImportPath(packages))
	return packages, nil
}

type byPackageImportPath []*build.Package

func (b byPackageImportPath) Len() int               { return len(b) }
func (b byPackageImportPath) Less(i int, j int) bool { return b[i].ImportPath < b[j].ImportPath }
func (b byPackageImportPath) Swap(i int, j int)      { b[i], b[j] = b[j], b[i] }
