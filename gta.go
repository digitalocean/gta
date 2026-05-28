/*
Copyright 2016 The gta AUTHORS. All rights reserved.

Use of this source code is governed by the Apache 2 license that can be found
in the LICENSE file.
*/
package gta

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/build"
	"go/scanner"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

var (
	// ErrNoDiffer is returned when there is no differ set on the GTA.
	ErrNoDiffer = errors.New("there is no differ set")
	// ErrNoPackager is returned when there is no packager set on the GTA.
	ErrNoPackager = errors.New("there is no packager set")
)

// Packages contains various detailed information about the structure of
// packages GTA has detected.
type Packages struct {
	// Dependencies contains a map of changed packages to their dependencies
	Dependencies map[string][]Package

	// Changes represents the changed files
	Changes []Package

	// AllChanges represents all packages that are dirty including the initial
	// changed packages.
	AllChanges []Package
}

type packagesJSON struct {
	Dependencies map[string][]string `json:"dependencies,omitempty"`
	Changes      []string            `json:"changes,omitempty"`
	AllChanges   []string            `json:"all_changes,omitempty"`
}

// MarshalJSON implements the json.Marshaler interface.
func (p *Packages) MarshalJSON() ([]byte, error) {
	s := packagesJSON{
		Dependencies: mapify(p.Dependencies),
		Changes:      stringify(p.Changes),
		AllChanges:   stringify(p.AllChanges),
	}
	return json.Marshal(s)
}

// UnmarshalJSON used by gtartifacts when providing a changed package list
// see `useChangedPackagesFrom()`
func (p *Packages) UnmarshalJSON(b []byte) error {
	s := new(packagesJSON)

	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	p.Dependencies = make(map[string][]Package)
	for k, v := range s.Dependencies {
		for _, vv := range v {
			p.Dependencies[k] = append(p.Dependencies[k], Package{ImportPath: vv})
		}
	}

	for _, v := range s.Changes {
		p.Changes = append(p.Changes, Package{ImportPath: v})
	}

	for _, v := range s.AllChanges {
		p.AllChanges = append(p.AllChanges, Package{ImportPath: v})
	}

	return nil
}

// A GTA provides a method of building dirty packages, and their dependent
// packages.
type GTA struct {
	differ                    Differ
	packager                  Packager
	prefixes                  []string
	tags                      []string
	roots                     []string
	includeTransitiveTestDeps bool
}

// New returns a new GTA with various options passed to New. Options will be
// applied in order so that later options can override earlier options.
func New(opts ...Option) (*GTA, error) {
	gta := &GTA{
		differ:                    NewGitDiffer(),
		includeTransitiveTestDeps: true,
	}

	for _, opt := range opts {
		err := opt(gta)
		if err != nil {
			return nil, err
		}
	}

	if gta.roots == nil {
		roots, err := toplevel()
		if err != nil {
			return nil, fmt.Errorf("could not get top level directory")
		}
		gta.roots = roots
	}

	// set the default packager after applying option so that the default
	// packager implementation does not load packages unnecessarily when the
	// packager is provided as an option.
	if gta.packager == nil {
		/*
			patterns, err := patternsFrom(gta.differ, gta.prefixes)
			if err != nil {
				return nil, err
			}

			gta.packager = NewPackager(patterns, gta.tags)
		*/

		// Cause NewPackager to return a packager that loads all packages by
		// passing a nil pattern.  This is important to ensure that all packages
		// are loaded and that nothing is skipped based on build tag constraints
		// when a file is changed. e.g. if a vendored file that is constrained to
		// Windows is changed, that package wouldn't load at all and trying to find
		// the package's dependencies would fail.
		gta.packager = NewPackager(nil, gta.tags)
	}

	return gta, nil
}

// ChangedPackages uses the differ and packager to build a map of changed root
// packages to their dependent packages where dependent is defined as "changed"
// as well due to their dependency to the changed packages. It returns the
// dependency graph, the changes differ detected and a set of all unique
// packages (including the changes).
//
// As an example: package "foo" is imported by packages "bar" and "qux". If
// "foo" has changed, it has two dependent packages, "bar" and "qux". The
// result would be then:
//
//	Dependencies = {"foo": ["bar", "qux"]}
//	Changes      = ["foo"]
//	AllChanges   = ["foo", "bar", "qux]
//
// Note that two different changed package might have the same dependent
// package. Below you see that both "foo" and "foo2" has changed. Each have
// "bar" because "bar" imports both "foo" and "foo2", i.e:
//
//	Dependencies = {"foo": ["bar", "qux"], "foo2" : ["afa", "bar", "qux"]}
//	Changes      = ["foo", "foo2"]
//	AllChanges   = ["foo", "foo2", "afa", "bar", "qux]
func (g *GTA) ChangedPackages() (*Packages, error) {
	paths, err := g.markedPackages()
	if err != nil {
		return nil, err
	}

	cp := &Packages{
		Dependencies: map[string][]Package{},
	}

	packageFromImport := func(path string) (*Package, error) {
		pkg, err := g.packager.PackageFromImport(path)
		if err != nil {
			return nil, err
		}

		return pkg, nil
	}

	// build our packages
	allChanges := map[string]Package{}
	for changed, marked := range paths {
		var packages []Package

		// add any dependents of the changed package; the changed package will be included in marked.
		for path, check := range marked {
			pkg := new(Package)
			pkg.ImportPath = path

			if check {
				pkg2, err := packageFromImport(path)
				if err != nil {
					return nil, err
				}
				pkg = pkg2
			}

			addPackage := func(pkg Package) {
				allChanges[pkg.ImportPath] = pkg
				if changed == pkg.ImportPath {
					cp.Changes = append(cp.Changes, pkg)
				} else {
					packages = append(packages, pkg)
				}
			}

			if hasPrefixIn(pkg.ImportPath, g.prefixes) {
				addPackage(*pkg)
			}
		}

		if len(packages) != 0 {
			sort.Sort(byPackageImportPath(packages))
			cp.Dependencies[changed] = packages
		}
	}

	for _, pkg := range allChanges {
		cp.AllChanges = append(cp.AllChanges, pkg)
	}
	sort.Sort(byPackageImportPath(cp.AllChanges))
	sort.Sort(byPackageImportPath(cp.Changes))

	return cp, nil
}

// markedPackages returns a map of maps. The outer map's key is the import path
// of a package that was changed according to g.differ. The inner maps' (i.e.
// the values of the outer map) keys are import paths of the dependents of the
// packages in respective key of the outer map. The inner maps' boolean values
// are true when the respective package exists and false when the respective
// package was deleted.
func (g *GTA) markedPackages() (map[string]map[string]bool, error) {
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

	// We build our set of initial dirty packages from the git diff. The map
	// value is true when the package was deleted. The map keys are package
	// import paths.
	changed := make(map[string]bool)
	embeddedChanged := make(map[string]struct{})
	onlyTestsAffected := make(map[string]struct{})
	onlyTestPackagesChanged := make(map[string]struct{})
	for abs, dir := range dirs {
		// TODO(bc): handle changes to go.mod when vendoring is not being used.

		// Add packages that embed the files of dir.
		for _, f := range dir.Files {
			// An embedded file may:
			//   1. reside in a directory that contains go files and
			//   2. reside in a directory that does not contain any go files
			//   3. be embedded by multiple packages
			// Therefore, do not try short-circuiting anything; just record that the
			// embedding packages are changed.
			for _, importPath := range g.packager.EmbeddedBy(filepath.Join(abs, f)) {
				embeddedChanged[importPath] = struct{}{}
				// Set the value to false, because the package is known to exist.
				changed[importPath] = false
			}
		}

		if isIgnoredByGo(abs, g.roots) {
			if !isTestData(abs) {
				continue
			}

			absAncestor := deepestUnignoredDir(abs, g.roots)
			if _, ok := dirs[absAncestor]; ok {
				// continue when the deepest unignored directory will be explicitly handled
				continue
			}

			// TODO(bc): take GOPATH / module root into account and don't try going above them?
			if absAncestor == "/" {
				continue
			}

			// set abs and dir to respective values to be evaluated.
			abs = absAncestor
			onlyTestsAffected[abs] = struct{}{}
			// Assume the directory exists; since it's not in the list of dirs, it
			// likely still exists. The only way it wouldn't would be is if the only
			// files in it were all deleted and it didn't directly contain any files.
			// It should be ok to assume it does exist even in that unlikely
			// situation.
			dir = Directory{
				Exists: true,
			}
		} else {
			if hasOnlyTestFilenames(dir.Files) {
				onlyTestsAffected[abs] = struct{}{}
			}
		}

		// Ignore deleted directories that did not contain files. Continue without
		// considering embedded files, because it is unknown whether a deleted file
		// was previously embedded.
		if !dir.Exists && !hasGoFile(dir.Files) {
			continue
		}

		pkg, err := g.packager.PackageFromDir(abs)
		if err != nil {
			switch err.(type) {
			case *build.NoGoError:
				if hasGoFile(dir.Files) {
					importPath, err := g.findImportPath(abs)
					if err != nil {
						continue
					}
					pkg.ImportPath = importPath

					changed[pkg.ImportPath] = true
					if _, ok := onlyTestsAffected[abs]; ok {
						onlyTestPackagesChanged[pkg.ImportPath] = struct{}{}
					}
					continue
				}
				// there are and were no buildable go files in this directory
				// so no dirty packages
				continue
			case scanner.ErrorList:
				// same, package is not buildable, so no dirty packages
				continue
			default:
				if !dir.Exists && hasGoFile(dir.Files) {
					importPath, err := g.findImportPath(abs)
					if err != nil {
						continue
					}

					changed[importPath] = true
					if _, ok := onlyTestsAffected[abs]; ok {
						onlyTestPackagesChanged[importPath] = struct{}{}
					}
					continue
				}
			}
			return nil, fmt.Errorf("pulling package information for %q, %v", abs, err)
		}

		// create a simple set of changed pkgs by import path. The packages that are tracked have at least one of the following properties:
		// * deleted (there's no need to check for this property here; it's already handled by this point)
		// * changed Go files
		// * tests are affected (e.g. files below a testdata directory)
		// * changed embedded files
		shouldMark := hasGoFile(dir.Files)
		if _, ok := onlyTestsAffected[abs]; ok {
			onlyTestPackagesChanged[pkg.ImportPath] = struct{}{}
			shouldMark = true
		}
		if _, ok := onlyTestPackagesChanged[pkg.ImportPath]; ok {
			shouldMark = true
		}

		if shouldMark {
			changed[pkg.ImportPath] = false
		}
	}

	// do not assume that only tests are affected if the package's embedded files
	// were changed. We do not have enough information to know whether the
	// embedded files are exclusively used by the tests, so assume that are used
	// by more than the tests.
	for k := range onlyTestPackagesChanged {
		if _, ok := embeddedChanged[k]; ok {
			delete(onlyTestPackagesChanged, k)
		}
	}

	// we build the dependent graph
	graph, err := g.packager.DependentGraph()
	if err != nil {
		return nil, fmt.Errorf("building dependency graph, %v", err)
	}

	// we also get the test-only dependent graph
	testOnlyGraph, err := g.packager.TestOnlyDependentGraph()
	if err != nil {
		return nil, fmt.Errorf("building test-only dependency graph, %v", err)
	}

	paths := map[string]map[string]bool{}
	for change := range changed {
		marked := make(map[string]bool)

		if _, ok := onlyTestPackagesChanged[change]; ok {
			marked[change] = !changed[change]
			paths[change] = marked
			continue
		}

		// Traverse dependents. When includeTransitiveTestDeps is true, test-only
		// edges are traversed the same as production edges (replicating the old
		// behavior where all imports were in a single reverse graph).
		var traverse func(node string)
		traverse = func(node string) {
			if marked[node] {
				return
			}
			marked[node] = true

			if edges, ok := graph.graph[node]; ok {
				for edge := range edges {
					traverse(edge)
				}
			}
			if g.includeTransitiveTestDeps {
				if edges, ok := testOnlyGraph.graph[node]; ok {
					for edge := range edges {
						traverse(edge)
					}
				}
			}
		}
		traverse(change)

		// When not traversing transitive test deps, still mark direct test-only
		// dependents (their tests need re-running, but we don't traverse further).
		if !g.includeTransitiveTestDeps {
			for testDep := range testOnlyGraph.graph[change] {
				marked[testDep] = true
			}
		}

		// clear the boolean value on the paths that no longer contain packages (i.e.
		// the Go files were deleted...).
		for importPath := range marked {
			if changed[importPath] {
				marked[importPath] = false
			}
		}

		paths[change] = marked
	}

	return paths, nil
}

var errImportPathNotFound = errors.New("could not find import path")

// findImportPath walks a directory up, trying to find an import path for
// parent directories.
func (g *GTA) findImportPath(abs string) (string, error) {
	base := filepath.Base(abs)
	parent := filepath.Dir(abs)

	if base == abs {
		return "", errImportPathNotFound
	}

	if !exists(abs) {
		//	recurse when the directory doesn't exist
		importPath, err := g.findImportPath(parent)
		if err != nil && err == errImportPathNotFound {
			return path.Join(importPath, base), err
		}
		return path.Join(importPath, base), nil
	}

	pkg, err := g.packager.PackageFromDir(abs)
	if err != nil {
		if _, ok := err.(*build.NoGoError); ok {
			pkg, err := g.packager.PackageFromEmptyDir(abs)
			if err == nil {
				return pkg.ImportPath, nil
			}
		}
		importPath, err := g.findImportPath(parent)
		return path.Join(importPath, base), err
	}

	return pkg.ImportPath, nil
}

type byPackageImportPath []Package

func (b byPackageImportPath) Len() int               { return len(b) }
func (b byPackageImportPath) Less(i int, j int) bool { return b[i].ImportPath < b[j].ImportPath }
func (b byPackageImportPath) Swap(i int, j int)      { b[i], b[j] = b[j], b[i] }

func stringify(pkgs []Package) []string {
	var out []string
	for _, pkg := range pkgs {
		out = append(out, pkg.ImportPath)
	}
	return out
}

func mapify(pkgs map[string][]Package) map[string][]string {
	out := map[string][]string{}
	for key, pkgs := range pkgs {
		out[key] = stringify(pkgs)
	}
	return out
}

func hasGoFile(files []string) bool {
	for _, fn := range files {
		if filepath.Ext(fn) == ".go" {
			return true
		}
	}
	return false
}

func hasPrefixIn(s string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func isIgnoredByGo(name string, roots []string) bool {
	for _, root := range roots {
		if root == name {
			return false
		}
	}

	base := filepath.Base(name)
	if base[0] == filepath.Separator {
		return false
	}

	// Avoid .foo, _foo, and testdata directory trees how the go tool does!
	// See https://github.com/golang/tools/blob/3a85b8d/go/buildutil/allpackages.go#L93
	// Above link is not guaranteed to work.
	if base == "" || base[0] == '.' || base[0] == '_' || base == "testdata" {
		return true
	}

	dir := filepath.Dir(name)
	if dir == "." {
		return false
	}

	return isIgnoredByGo(filepath.Dir(name), roots)
}

func isTestData(name string) bool {
	base := filepath.Base(name)
	if base[0] == filepath.Separator {
		return false
	}

	if name == "testdata" || base == "testdata" {
		return true
	}

	dir := filepath.Dir(name)
	if dir == "." {
		return false
	}

	return isTestData(filepath.Dir(name))
}

func deepestUnignoredDir(name string, roots []string) string {
	if name == "." || name == "/" {
		return name
	}

	if isIgnoredByGo(name, roots) {
		return deepestUnignoredDir(filepath.Dir(name), roots)
	}

	return name
}

func hasOnlyTestFilenames(sl []string) bool {
	for _, v := range sl {
		if !strings.HasSuffix(v, "_test.go") {
			return false
		}
	}
	return true
}

func toplevel() ([]string, error) {
	if os.Getenv("GO111MODULE") == "off" {
		return gopaths()
	}

	// In workspace mode, go list -m -f '{{.Dir}}' outputs one line per workspace
	// module, producing a corrupt multi-line string when TrimSpace'd. Use
	// go env GOWORK instead, which always returns a single path.
	root, ok, err := workspaceroot()
	if err != nil {
		return nil, err
	}
	if ok {
		return []string{root}, nil
	}

	root, err = moduleroot()
	if err != nil {
		return nil, err
	}
	return []string{root}, nil
}

func gopaths() ([]string, error) {
	cmd := exec.Command("go", "env", "GOPATH")
	b, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("could get not get GOPATH: %w", err)
	}

	var roots []string
	for _, v := range strings.Split(string(b), string(os.PathListSeparator)) {
		roots = append(roots, strings.TrimSpace(v))
	}
	return roots, nil
}

func moduleroot() (string, error) {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}")
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("could get not get module root: %w", err)
	}

	return strings.TrimSpace(string(b)), nil
}

// workspaceroot returns the directory containing the active go.work file and
// true when Go workspace mode is active.
func workspaceroot() (string, bool, error) {
	cmd := exec.Command("go", "env", "GOWORK")
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", false, fmt.Errorf("go env GOWORK: %w", err)
	}
	dir, ok := parseGOWORK(string(b))
	return dir, ok, nil
}

// parseGOWORK interprets the raw output of `go env GOWORK`.
// "off" indicates workspace mode was explicitly disabled via GOWORK=off.
func parseGOWORK(output string) (string, bool) {
	gowork := strings.TrimSpace(output)
	if gowork == "" || gowork == "off" {
		return "", false
	}
	return filepath.Dir(gowork), true
}
