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
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/tools/go/packages/packagestest"
)

var _ Differ = &testDiffer{}

type testDiffer struct {
	diff map[string]Directory
}

func (t *testDiffer) Diff() (map[string]Directory, error) {
	return t.diff, nil
}

func (t *testDiffer) DiffFiles() (map[string]bool, error) {
	panic("not implemented")
}

var _ Packager = &testPackager{}

type testPackager struct {
	dirs2Imports map[string]string
	graph        *Graph
	errs         map[string]error
}

func (t *testPackager) PackageFromDir(a string) (*Package, error) {
	// we pass back an err
	err, eok := t.errs[a]
	if eok {
		return nil, err
	}

	path, ok := t.dirs2Imports[a]
	if !ok {
		return nil, errors.New("dir not found")
	}

	return &Package{
		ImportPath: path,
	}, nil
}

func (t *testPackager) PackageFromEmptyDir(a string) (*Package, error) {
	return nil, errors.New("not implemented")
}

func (t *testPackager) PackageFromImport(a string) (*Package, error) {
	for _, v := range t.dirs2Imports {
		if a == v {
			return &Package{
				ImportPath: a,
			}, nil
		}
	}
	return nil, errors.New("pkg not found")
}

func (t *testPackager) DependentGraph() (*Graph, error) {
	return t.graph, nil
}

func (t *testPackager) TestOnlyDependentGraph() (*Graph, error) {
	// For legacy tests, return an empty graph
	return &Graph{graph: make(map[string]map[string]bool)}, nil
}

func (_ *testPackager) EmbeddedBy(_ string) []string {
	return nil
}

func TestGTA(t *testing.T) {
	// A depends on B depends on C
	// dirC is dirty, we expect them all to be marked
	difr := &testDiffer{
		diff: map[string]Directory{
			"dirC": Directory{
				Exists: true,
				Files:  []string{"foo.go"},
			},
		},
	}

	graph := &Graph{
		graph: map[string]map[string]bool{
			"C": map[string]bool{
				"B": true,
			},
			"B": map[string]bool{
				"A": true,
			},
		},
	}

	pkgr := &testPackager{
		dirs2Imports: map[string]string{
			"dirA": "A",
			"dirB": "B",
			"dirC": "C",
		},
		graph: graph,
		errs:  make(map[string]error),
	}
	want := []Package{
		Package{ImportPath: "A"},
		Package{ImportPath: "B"},
		Package{ImportPath: "C"},
	}

	gta, err := New(SetDiffer(difr), SetPackager(pkgr))
	if err != nil {
		t.Fatal(err)
	}

	pkgs, err := gta.ChangedPackages()
	if err != nil {
		t.Fatal(err)
	}

	got := pkgs.AllChanges

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("(-want, +got)\n%s", diff)
	}
}

func TestGTA_TraversalDepth(t *testing.T) {
	// Dependency chain: A depends on B depends on C depends on D
	// (in reverse-graph terms: D→C→B→A)
	// dirD is dirty.
	difr := &testDiffer{
		diff: map[string]Directory{
			"dirD": {Exists: true, Files: []string{"d.go"}},
		},
	}

	graph := &Graph{
		graph: map[string]map[string]bool{
			"D": {"C": true},
			"C": {"B": true},
			"B": {"A": true},
		},
	}

	pkgr := &testPackager{
		dirs2Imports: map[string]string{
			"dirA": "A",
			"dirB": "B",
			"dirC": "C",
			"dirD": "D",
		},
		graph: graph,
		errs:  make(map[string]error),
	}

	tests := []struct {
		name           string
		traversalDepth int
		wantAllChanges []Package
		wantErr        bool
	}{
		{
			name:           "depth 0 (unlimited) traverses full chain",
			traversalDepth: 0,
			wantAllChanges: []Package{
				{ImportPath: "A"},
				{ImportPath: "B"},
				{ImportPath: "C"},
				{ImportPath: "D"},
			},
		},
		{
			name:           "depth 1 marks only the changed package and its direct dependents",
			traversalDepth: 1,
			wantAllChanges: []Package{
				{ImportPath: "C"},
				{ImportPath: "D"},
			},
		},
		{
			name:           "depth 2 marks two hops from the changed package",
			traversalDepth: 2,
			wantAllChanges: []Package{
				{ImportPath: "B"},
				{ImportPath: "C"},
				{ImportPath: "D"},
			},
		},
		{
			name:           "depth 3 marks three hops from the changed package",
			traversalDepth: 3,
			wantAllChanges: []Package{
				{ImportPath: "A"},
				{ImportPath: "B"},
				{ImportPath: "C"},
				{ImportPath: "D"},
			},
		},
		{
			name:           "negative depth returns an error",
			traversalDepth: -1,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := New(SetDiffer(difr), SetPackager(pkgr), SetTraversalDepth(tt.traversalDepth))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			pkgs, err := g.ChangedPackages()
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(tt.wantAllChanges, pkgs.AllChanges); diff != "" {
				t.Errorf("AllChanges (-want, +got)\n%s", diff)
			}
		})
	}
}

func TestGTA_ChangedPackages(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		// A depends on B depends on C
		// D depends on B
		// E depends on F depends on G

		difr := &testDiffer{
			diff: map[string]Directory{
				"dirC": Directory{Exists: true, Files: []string{"c.go"}},
				"dirH": Directory{Exists: true, Files: []string{"h.go"}},
			},
		}

		graph := &Graph{
			graph: map[string]map[string]bool{
				"C": map[string]bool{
					"B": true,
				},
				"B": map[string]bool{
					"A": true,
					"D": true,
				},
				"G": map[string]bool{
					"F": true,
				},
				"F": map[string]bool{
					"E": true,
				},
			},
		}

		pkgr := &testPackager{
			dirs2Imports: map[string]string{
				"dirA": "A",
				"dirB": "B",
				"dirC": "C",
				"dirD": "D",
				"dirF": "E",
				"dirG": "F",
				"dirH": "G",
			},
			graph: graph,
			errs:  make(map[string]error),
		}

		want := &Packages{
			Dependencies: map[string][]Package{
				"C": []Package{
					{ImportPath: "A"},
					{ImportPath: "B"},
					{ImportPath: "D"},
				},
				"G": []Package{
					{ImportPath: "E"},
					{ImportPath: "F"},
				},
			},
			Changes: []Package{
				{ImportPath: "C"},
				{ImportPath: "G"},
			},
			AllChanges: []Package{
				{ImportPath: "A"},
				{ImportPath: "B"},
				{ImportPath: "C"},
				{ImportPath: "D"},
				{ImportPath: "E"},
				{ImportPath: "F"},
				{ImportPath: "G"},
			},
		}

		gta, err := New(SetDiffer(difr), SetPackager(pkgr))
		if err != nil {
			t.Fatal(err)
		}

		got, err := gta.ChangedPackages()
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(got, want); diff != "" {
			t.Errorf("(-want, +got)\n%s", diff)
		}
	})

	const testModule string = "gta.test"
	// testChangedPackages executes ChangedPackages for each of the exporters and
	// makes sure the return values match expectations. diff is a map of
	// directory name fragments (i.e a relative directory sans ./) to Directory
	// values that will be expanded and provided as a differ via testDiffer.
	// shouldRemoveFile is a function that returns a boolean value indicating
	// whether a file identified by a filename fragment should be deleted. want
	// is the expected value from ChangedPackages().
	testChangedPackages := func(t *testing.T, diff map[string]Directory, shouldRemoveFile func(string) bool, want *Packages) {
		t.Helper()

		packagestest.TestAll(t, func(t *testing.T, exporter packagestest.Exporter) {
			t.Helper()

			e := packagestest.Export(t, exporter, []packagestest.Module{
				{
					Name:  testModule,
					Files: packagestest.MustCopyFileTree(filepath.Join("testdata", "gtatest")),
				},
			})

			t.Cleanup(e.Cleanup)

			// create a new map from diff
			m := make(map[string]Directory)
			for k, v := range diff {
				// expand keys to the absolute path
				m[exporter.Filename(e, testModule, k)] = v

				// delete v if the diff says it shouldn't exist.
				if !v.Exists {
					err := os.RemoveAll(exporter.Filename(e, testModule, k))
					if err != nil {
						t.Fatal(fmt.Errorf("could not remove %s: %w", k, err))
					}
				} else {
					if shouldRemoveFile != nil {
						for _, file := range v.Files {
							fragment := path.Join(k, file)
							if !shouldRemoveFile(fragment) {
								continue
							}
							err := os.Remove(exporter.Filename(e, testModule, fragment))
							if err != nil {
								t.Fatal(fmt.Errorf("could not remove %s: %w", fragment, err))
							}
						}
					}
				}
			}
			difr := &testDiffer{
				diff: m,
			}

			qualifyPackages := func(pkgs []Package) []Package {
				qualified := make([]Package, len(pkgs))
				for i, pkg := range pkgs {
					pkg.ImportPath = fmt.Sprintf("%s/%s", testModule, pkg.ImportPath)
					// deleted packages should have an empty Dir value and should not be
					// expanded.
					if pkg.Dir != "" {
						pkg.Dir = exporter.Filename(e, testModule, pkg.Dir)
					}
					qualified[i] = pkg
				}

				return qualified
			}

			deps := make(map[string][]Package)
			for k, v := range want.Dependencies {
				v = qualifyPackages(v)
				deps[fmt.Sprintf("%s/%s", testModule, k)] = v
			}

			qualifiedWant := new(Packages)
			qualifiedWant.Dependencies = deps
			qualifiedWant.Changes = qualifyPackages(want.Changes)
			qualifiedWant.AllChanges = qualifyPackages(want.AllChanges)

			popd := chdir(t, exporter.Filename(e, testModule, ""))
			t.Cleanup(popd)

			cfg := newLoadConfig(nil)
			e.Config.Mode = cfg.Mode
			e.Config.BuildFlags = cfg.BuildFlags
			e.Config.Tests = cfg.Tests

			// the default build.Context uses GOPATH as its set at initialization and
			// it must be overridden for each test.
			for _, v := range e.Config.Env {
				sl := strings.SplitN(v, "=", 2)
				if sl[0] != "GOPATH" {
					continue
				}

				// reset the default build.Context's value after the test completes.
				defer func(v string) {
					build.Default.GOPATH = v
				}(build.Default.GOPATH)

				build.Default.GOPATH = sl[1]
			}
			defer AllSetenv(t, e.Config.Env)()

			sut, err := New(SetDiffer(difr), SetPackager(newPackager(e.Config, build.Default, []string{testModule + "/"})))
			if err != nil {
				t.Fatal(err)
			}

			got, err := sut.ChangedPackages()
			if err != nil {
				t.Fatal(err)
			}

			packagesEqual := func(pkg1, pkg2 Package) bool {
				return pkg1.ImportPath == pkg2.ImportPath && (len(pkg1.Dir) == 0) == (len(pkg2.Dir) == 0)
			}
			if diff := cmp.Diff(qualifiedWant, got, cmp.Comparer(packagesEqual)); diff != "" {
				t.Errorf("(-want, +got)\n%s", diff)
			}
		})
	}

	// alwaysRemove is a convenience function to pass to testChangedPackages to
	// cause every file in the diff to be removed from disk.
	alwaysRemove := func(_ string) bool {
		// delete all the go files in diff.
		return true
	}
	t.Run("proper deletion", func(t *testing.T) {
		// TODO(bc): figure out how to delete the files
		t.Run("go files only", func(t *testing.T) {
			diff := map[string]Directory{
				"gofilesdeleted":       {Exists: true, Files: []string{"gofilesdeleted.go"}},
				"gofilesdeletedclient": {Exists: true, Files: []string{"gofilesdeletedclient.go"}},
			}

			want := &Packages{
				Dependencies: map[string][]Package{},
				Changes: []Package{
					{ImportPath: "gofilesdeleted"},
					{ImportPath: "gofilesdeletedclient"},
				},
				AllChanges: []Package{
					{ImportPath: "gofilesdeleted"},
					{ImportPath: "gofilesdeletedclient"},
				},
			}

			shouldDelete := func(fragment string) bool {
				// delete all the go files in diff.
				return true
			}

			testChangedPackages(t, diff, shouldDelete, want)
		})

		t.Run("directory", func(t *testing.T) {
			diff := map[string]Directory{
				"deleted":       {Exists: false, Files: []string{"deleted.go"}},
				"deletedclient": {Exists: false, Files: []string{"deletedclient.go"}},
			}

			want := &Packages{
				Dependencies: map[string][]Package{},
				Changes: []Package{
					{ImportPath: "deleted"},
					{ImportPath: "deletedclient"},
				},
				AllChanges: []Package{
					{ImportPath: "deleted"},
					{ImportPath: "deletedclient"},
				},
			}

			testChangedPackages(t, diff, nil, want)
		})
	})

	t.Run("partial deletion", func(t *testing.T) {
		t.Run("go files only", func(t *testing.T) {
			diff := map[string]Directory{
				"gofilesdeleted": {Exists: true, Files: []string{"gofilesdeleted.go"}},
			}

			want := &Packages{
				Dependencies: map[string][]Package{
					"gofilesdeleted": {
						{ImportPath: "gofilesdeletedclient", Dir: "gofilesdeletedclient"},
					},
				},
				Changes: []Package{
					{ImportPath: "gofilesdeleted"},
				},
				AllChanges: []Package{
					{ImportPath: "gofilesdeleted"},
					{ImportPath: "gofilesdeletedclient", Dir: "gofilesdeletedclient"},
				},
			}

			testChangedPackages(t, diff, alwaysRemove, want)
		})

		t.Run("directory", func(t *testing.T) {
			diff := map[string]Directory{
				"deleted": {Exists: false, Files: []string{"deleted.go"}},
			}

			want := &Packages{
				Dependencies: map[string][]Package{
					"deleted": {
						{ImportPath: "deletedclient", Dir: "deletedClient"},
					},
				},
				Changes: []Package{
					{ImportPath: "deleted"},
				},
				AllChanges: []Package{
					{ImportPath: "deleted"},
					{ImportPath: "deletedclient", Dir: "deletedclient"},
				},
			}

			testChangedPackages(t, diff, nil, want)
		})
	})

	t.Run("change dependency", func(t *testing.T) {
		diff := map[string]Directory{
			"foo": {Exists: true, Files: []string{"foo.go"}},
		}

		want := &Packages{
			Dependencies: map[string][]Package{
				"foo": {
					{ImportPath: "fooclient", Dir: "fooclient"},
					{ImportPath: "fooclientclient", Dir: "fooclientclient"},
				},
			},
			Changes: []Package{
				{ImportPath: "foo", Dir: "foo"},
			},
			AllChanges: []Package{
				{ImportPath: "foo", Dir: "foo"},
				{ImportPath: "fooclient", Dir: "fooclient"},
				{ImportPath: "fooclientclient", Dir: "fooclientclient"},
			},
		}
		testChangedPackages(t, diff, nil, want)
	})

	t.Run("change transitive dependency", func(t *testing.T) {
		diff := map[string]Directory{
			"foo": {Exists: true, Files: []string{"foo.go"}},
		}

		want := &Packages{
			Dependencies: map[string][]Package{
				"foo": {
					{ImportPath: "fooclient", Dir: "fooclient"},
					{ImportPath: "fooclientclient", Dir: "fooclientclient"},
				},
			},
			Changes: []Package{
				{ImportPath: "foo", Dir: "foo"},
			},
			AllChanges: []Package{
				{ImportPath: "foo", Dir: "foo"},
				{ImportPath: "fooclient", Dir: "fooclient"},
				{ImportPath: "fooclientclient", Dir: "fooclientclient"},
			},
		}

		testChangedPackages(t, diff, nil, want)
	})

	t.Run("change transitive dependency test", func(t *testing.T) {
		diff := map[string]Directory{
			"foo":       {Exists: true, Files: []string{"foo.go"}},
			"fooclient": {Exists: true, Files: []string{"fooclient_test.go"}},
		}

		want := &Packages{
			Dependencies: map[string][]Package{
				"foo": {
					{ImportPath: "fooclient", Dir: "fooclient"},
					{ImportPath: "fooclientclient", Dir: "fooclientclient"},
				},
			},
			Changes: []Package{
				{ImportPath: "foo", Dir: "foo"},
				{ImportPath: "fooclient", Dir: "fooclient"},
			},
			AllChanges: []Package{
				{ImportPath: "foo", Dir: "foo"},
				{ImportPath: "fooclient", Dir: "fooclient"},
				{ImportPath: "fooclientclient", Dir: "fooclientclient"},
			},
		}

		testChangedPackages(t, diff, nil, want)
	})

	t.Run("change no dependency", func(t *testing.T) {
		diff := map[string]Directory{
			"unimported": {Exists: true, Files: []string{"unimported.go"}},
		}

		want := &Packages{
			Dependencies: map[string][]Package{},
			Changes: []Package{
				{ImportPath: "unimported", Dir: "unimported"},
			},
			AllChanges: []Package{
				{ImportPath: "unimported", Dir: "unimported"},
			},
		}

		testChangedPackages(t, diff, nil, want)
	})
	t.Run("change external", func(t *testing.T) {
		diff := map[string]Directory{
			"foo": {Exists: true, Files: []string{"foo.go", "foo_test.go"}},
		}

		want := &Packages{
			Dependencies: map[string][]Package{
				"foo": {
					{ImportPath: "fooclient", Dir: "fooclient"},
					{ImportPath: "fooclientclient", Dir: "fooclientclient"},
				},
			},
			Changes: []Package{
				{ImportPath: "foo", Dir: "foo"},
			},
			AllChanges: []Package{
				{ImportPath: "foo", Dir: "foo"},
				{ImportPath: "fooclient", Dir: "fooclient"},
				{ImportPath: "fooclientclient", Dir: "fooclientclient"},
			},
		}

		testChangedPackages(t, diff, nil, want)
	})
	t.Run("change test", func(t *testing.T) {
		diff := map[string]Directory{
			"foo": {Exists: true, Files: []string{"foo_test.go"}},
		}

		want := &Packages{
			Dependencies: map[string][]Package{},
			Changes: []Package{
				{ImportPath: "foo", Dir: "foo"},
			},
			AllChanges: []Package{
				{ImportPath: "foo", Dir: "foo"},
			},
		}

		testChangedPackages(t, diff, nil, want)
	})

	// "bar_test" is a package with a name ending in "_test", not a test
	// package, and ensuring the weird name is still found is the point of this
	// test.
	//
	// fooclient imports bar_test only in its test file, making it a test-only
	// dependent. With -test-transitive=true (default), we traverse test-only
	// dependents and their production dependents, so fooclientclient is
	// included.
	t.Run("change badly named package", func(t *testing.T) {
		diff := map[string]Directory{
			"bar_test": {Exists: true, Files: []string{"util.go"}},
		}

		want := &Packages{
			Dependencies: map[string][]Package{
				"bar_test": {
					{ImportPath: "fooclient", Dir: "fooclient"},
					{ImportPath: "fooclientclient", Dir: "fooclientclient"},
				},
			},
			Changes: []Package{
				{ImportPath: "bar_test", Dir: "bar_test"},
			},
			AllChanges: []Package{
				{ImportPath: "bar_test", Dir: "bar_test"},
				{ImportPath: "fooclient", Dir: "fooclient"},
				{ImportPath: "fooclientclient", Dir: "fooclientclient"},
			},
		}

		testChangedPackages(t, diff, nil, want)
	})
	t.Run("change embedded file", func(t *testing.T) {
		diff := map[string]Directory{
			"embed": {Exists: true, Files: []string{"embed.go"}},
		}

		want := &Packages{
			Dependencies: map[string][]Package{
				"embed": {
					{ImportPath: "embedclient", Dir: "embedclient"},
				},
			},
			Changes: []Package{
				{ImportPath: "embed", Dir: "embed"},
			},
			AllChanges: []Package{
				{ImportPath: "embed", Dir: "embed"},
				{ImportPath: "embedclient", Dir: "embedclient"},
			},
		}

		testChangedPackages(t, diff, nil, want)
	})

	t.Run("change constrained package", func(t *testing.T) {
		diff := map[string]Directory{
			"constrained": {Exists: true, Files: []string{"constrained.go"}},
		}

		want := &Packages{
			Dependencies: map[string][]Package{},
			Changes: []Package{
				{ImportPath: "constrained"},
			},
			AllChanges: []Package{
				{ImportPath: "constrained"},
			},
		}

		testChangedPackages(t, diff, nil, want)
	})

	t.Run("change non-go file", func(t *testing.T) {
		diff := map[string]Directory{
			"embed":      {Exists: true, Files: []string{"README.md"}},
			"unimported": {Exists: true, Files: []string{"unimported.go"}},
		}

		want := &Packages{
			Dependencies: map[string][]Package{},
			Changes: []Package{
				{ImportPath: "unimported", Dir: "unimported"},
			},
			AllChanges: []Package{
				{ImportPath: "unimported", Dir: "unimported"},
			},
		}

		testChangedPackages(t, diff, nil, want)
	})

	// Test-only dependencies propagate through the reverse dependency graph
	// when -test-transitive=true (the default).
	//
	// Test data setup:
	//   - testmock: a mock package that is only used in tests
	//   - usesmock: imports testmock only in its test file, usesmock_test.go
	//   - usesmockclient: imports usesmock for production, does not import testmock
	//
	// When testmock changes with -test-transitive=true, usesmock is found affected
	// because its tests use testmock, and usesmockclient is also found affected
	// because it imports usesmock (and we traverse all dependents when
	// -test-transitive=true).
	//
	t.Run("change test-only mock package affects production dependents with test=true", func(t *testing.T) {
		diff := map[string]Directory{
			"testmock": {Exists: true, Files: []string{"testmock.go"}},
		}

		want := &Packages{
			Dependencies: map[string][]Package{
				"testmock": {
					{ImportPath: "usesmock", Dir: "usesmock"},
					{ImportPath: "usesmockclient", Dir: "usesmockclient"},
				},
			},
			Changes: []Package{
				{ImportPath: "testmock", Dir: "testmock"},
			},
			AllChanges: []Package{
				{ImportPath: "testmock", Dir: "testmock"},
				{ImportPath: "usesmock", Dir: "usesmock"},
				{ImportPath: "usesmockclient", Dir: "usesmockclient"},
			},
		}

		testChangedPackages(t, diff, nil, want)
	})
}

func TestGTA_Prefix(t *testing.T) {
	// A depends on B and foo
	// B depends on C and bar
	// C depends on qux
	difr := &testDiffer{
		diff: map[string]Directory{
			"dirB":   Directory{Exists: true},
			"dirC":   Directory{Exists: true},
			"dirFoo": Directory{Exists: true},
		},
	}

	graph := &Graph{
		graph: map[string]map[string]bool{
			"C": map[string]bool{
				"B": true,
			},
			"B": map[string]bool{
				"A": true,
			},
			"foo": map[string]bool{
				"A": true,
			},
			"bar": map[string]bool{
				"B": true,
			},
			"qux": map[string]bool{
				"C": true,
			},
		},
	}

	pkgr := &testPackager{
		dirs2Imports: map[string]string{
			"dirA":   "A",
			"dirB":   "B",
			"dirC":   "C",
			"dirFoo": "foo",
			"dirBar": "bar",
			"dirQux": "qux",
		},
		graph: graph,
		errs:  make(map[string]error),
	}
	want := []Package{
		Package{ImportPath: "C"},
		Package{ImportPath: "foo"},
	}

	gta, err := New(SetDiffer(difr), SetPackager(pkgr), SetPrefixes("foo", "C"))
	if err != nil {
		t.Fatal(err)
	}

	pkgs, err := gta.ChangedPackages()
	if err != nil {
		t.Fatal(err)
	}

	got := pkgs.AllChanges

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("(-want, +got)\n%s", diff)
	}
}

func TestNoBuildableGoFiles(t *testing.T) {
	// we have changes but they don't belong to any dirty golang files, so no dirty packages
	const dir = "docs"
	difr := &testDiffer{
		diff: map[string]Directory{
			dir: Directory{},
		},
	}

	pkgr := &testPackager{
		errs: map[string]error{
			dir: &build.NoGoError{
				Dir: dir,
			},
		},
	}

	var want []Package

	gta, err := New(SetDiffer(difr), SetPackager(pkgr))
	if err != nil {
		t.Fatal(err)
	}

	pkgs, err := gta.ChangedPackages()
	if err != nil {
		t.Fatal(err)
	}

	got := pkgs.AllChanges

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("(-want, +got)\n%s", diff)
	}
}

func TestSpecialCaseDirectory(t *testing.T) {
	// We want to ignore the special case directory testdata for all but the
	// package that contains the testdata directory.
	const (
		special1 = "specia/case/testdata"
		special2 = "specia/case/testdata/multi"
	)
	difr := &testDiffer{
		diff: map[string]Directory{
			special1: Directory{Exists: true},
			special2: Directory{Exists: true},
			"dirC":   Directory{Exists: true, Files: []string{"c.go"}},
		},
	}
	graph := &Graph{
		graph: map[string]map[string]bool{
			"C": map[string]bool{
				"B": true,
			},
			"B": map[string]bool{
				"A": true,
			},
			"specia/case": map[string]bool{
				"D": true,
			},
		},
	}

	pkgr := &testPackager{
		dirs2Imports: map[string]string{
			"dirA":        "A",
			"dirB":        "B",
			"dirC":        "C",
			"dirD":        "D",
			"specia/case": "specia/case",
		},
		graph: graph,
		errs: map[string]error{
			special1: &build.NoGoError{
				Dir: special1,
			},
			special2: &build.NoGoError{
				Dir: special2,
			},
		},
	}

	want := []Package{
		Package{ImportPath: "A"},
		Package{ImportPath: "B"},
		Package{ImportPath: "C"},
		Package{ImportPath: "specia/case"},
	}

	gta, err := New(SetDiffer(difr), SetPackager(pkgr))
	if err != nil {
		t.Fatal(err)
	}

	pkgs, err := gta.ChangedPackages()
	if err != nil {
		t.Fatal(err)
	}

	got := pkgs.AllChanges

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("(-want, +got)\n%s", diff)
	}
}

func TestUnmarshalJSON(t *testing.T) {
	want := &Packages{
		Dependencies: map[string][]Package{
			"do/tools/build/gta": []Package{
				{
					ImportPath: "do/tools/build/gta/cmd/gta",
				},
				{
					ImportPath: "do/tools/build/gtartifacts",
				},
			},
		},
		Changes: []Package{
			{
				ImportPath: "do/teams/compute/octopus",
			},
		},
		AllChanges: []Package{
			{
				ImportPath: "do/teams/compute/octopus",
			},
		},
	}

	in := []byte(`{"dependencies":{"do/tools/build/gta":["do/tools/build/gta/cmd/gta","do/tools/build/gtartifacts"]},"changes":["do/teams/compute/octopus"],"all_changes":["do/teams/compute/octopus"]}`)

	got := new(Packages)
	err := json.Unmarshal(in, got)
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("(-want, +got)\n%s", diff)
	}
}

func TestJSONRoundtrip(t *testing.T) {
	want := &Packages{
		Dependencies: map[string][]Package{
			"do/tools/build/gta": []Package{
				{
					ImportPath: "do/tools/build/gta/cmd/gta",
				},
				{
					ImportPath: "do/tools/build/gtartifacts",
				},
			},
		},
		Changes: []Package{
			{
				ImportPath: "do/teams/compute/octopus",
			},
		},
		AllChanges: []Package{
			{
				ImportPath: "do/teams/compute/octopus",
			},
		},
	}

	b, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}

	got := new(Packages)
	err = json.Unmarshal(b, got)
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("(-want, +got)\n%s", diff)
	}
}

func TestIsIgnoredByGo(t *testing.T) {
	tests := []struct {
		in       string
		expected bool
	}{
		{
			in:       "/",
			expected: false,
		}, {
			in:       "/foo",
			expected: false,
		}, {
			in:       "/foo/bar",
			expected: false,
		}, {
			in:       "foo",
			expected: false,
		}, {
			in:       "testdata",
			expected: true,
		}, {
			in:       "/testdata",
			expected: true,
		}, {
			in:       "/foo/testdata",
			expected: true,
		}, {
			in:       "foo/testdata/bar",
			expected: true,
		}, {
			in:       "/foo/_bar",
			expected: true,
		}, {
			in:       "/foo/.bar",
			expected: true,
		}, {
			in:       "foo/_bar/quux",
			expected: true,
		}, {
			in:       "/foo/.bar/quux",
			expected: true,
		}, {
			in:       "/foo/_bar/baz",
			expected: false,
		},
	}
	for _, tt := range tests {
		got := isIgnoredByGo(tt.in, []string{"/", "/foo/_bar/baz"})
		if want := tt.expected; got != want {
			t.Errorf("isIgnoredByGoBuild(%q) = %v; want %v", tt.in, got, want)
		}
	}

}

func TestDeepestUnignoredDir(t *testing.T) {
	tests := []struct {
		in       string
		expected string
	}{
		{
			in:       "/",
			expected: "/",
		}, {
			in:       "/foo",
			expected: "/foo",
		}, {
			in:       "/foo/bar",
			expected: "/foo/bar",
		}, {
			in:       "foo",
			expected: "foo",
		}, {
			in:       "testdata",
			expected: ".",
		}, {
			in:       "/testdata",
			expected: "/",
		}, {
			in:       "/foo/testdata",
			expected: "/foo",
		}, {
			in:       "foo/testdata/bar",
			expected: "foo",
		}, {
			in:       "/foo/_bar",
			expected: "/foo",
		}, {
			in:       "/foo/.bar",
			expected: "/foo",
		}, {
			in:       "foo/_bar/quux",
			expected: "foo",
		}, {
			in:       "/foo/.bar/quux",
			expected: "/foo",
		}, {
			in:       "/foo/bar/testdata/quux/_baz",
			expected: "/foo/bar",
		},
	}
	for _, tt := range tests {
		got := deepestUnignoredDir(tt.in, []string{"/"})
		if want := tt.expected; got != want {
			t.Errorf("deepestUnignoredDir(%q) = %v; want %v", tt.in, got, want)
		}
	}
}

func TestParseGOWORK(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		input   string
		wantDir string
		wantOK  bool
	}{
		"active workspace": {
			input:   "/home/user/project/go.work\n",
			wantDir: "/home/user/project",
			wantOK:  true,
		},
		"explicitly disabled": {
			input:   "off\n",
			wantDir: "",
			wantOK:  false,
		},
		"empty output": {
			input:   "",
			wantDir: "",
			wantOK:  false,
		},
		"whitespace only": {
			input:   "   \n",
			wantDir: "",
			wantOK:  false,
		},
		"workspace at filesystem root": {
			input:   "/go.work\n",
			wantDir: "/",
			wantOK:  true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			gotDir, gotOK := parseGOWORK(tc.input)
			if gotDir != tc.wantDir || gotOK != tc.wantOK {
				t.Errorf("parseGOWORK(%q) = (%q, %v), want (%q, %v)",
					tc.input, gotDir, gotOK, tc.wantDir, tc.wantOK)
			}
		})
	}
}

func TestWorkspaceroot(t *testing.T) {
	tmpDir := t.TempDir()

	cases := map[string]struct {
		gowork  string
		wantDir string
		wantOK  bool
	}{
		"active workspace": {
			gowork:  filepath.Join(tmpDir, "go.work"),
			wantDir: tmpDir,
			wantOK:  true,
		},
		"disabled":     {gowork: "off"},
		"no workspace": {gowork: ""},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("GOWORK", tc.gowork)
			gotDir, gotOK, err := workspaceroot()
			if err != nil {
				t.Fatalf("workspaceroot() error: %v", err)
			}
			if gotDir != tc.wantDir || gotOK != tc.wantOK {
				t.Errorf("workspaceroot() = (%q, %v), want (%q, %v)",
					gotDir, gotOK, tc.wantDir, tc.wantOK)
			}
		})
	}
}

func TestSetRootsOption(t *testing.T) {
	t.Parallel()

	root := "/fake/workspace/root"
	g, err := New(
		SetRoots(root),
		SetDiffer(&testDiffer{}),
		SetPackager(&testPackager{
			dirs2Imports: map[string]string{},
			graph:        &Graph{graph: map[string]map[string]bool{}},
		}),
	)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if len(g.roots) != 1 || g.roots[0] != root {
		t.Errorf("roots = %v, want [%q]", g.roots, root)
	}
}
