package gta

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestPackageContextImplementsPackager(t *testing.T) {
	var sut interface{} = new(packageContext)
	if _, ok := sut.(Packager); !ok {
		t.Error("expected to implement Packager")
	}
}

func TestIsLocalPackage(t *testing.T) {
	tests := []struct {
		name              string
		modulesNamesByDir map[string]string
		importPath        string
		want              bool
	}{
		{
			name:              "local exact match",
			modulesNamesByDir: map[string]string{"/repo": "mymod"},
			importPath:        "mymod",
			want:              true,
		},
		{
			name:              "local subpackage",
			modulesNamesByDir: map[string]string{"/repo": "mymod"},
			importPath:        "mymod/pkg/foo",
			want:              true,
		},
		{
			name:              "external package",
			modulesNamesByDir: map[string]string{"/repo": "mymod"},
			importPath:        "golang.org/x/text",
			want:              false,
		},
		{
			name:              "stdlib package",
			modulesNamesByDir: map[string]string{"/repo": "mymod"},
			importPath:        "fmt",
			want:              false,
		},
		{
			name:              "similar prefix no match",
			modulesNamesByDir: map[string]string{"/repo": "mymod"},
			importPath:        "mymod2/foo",
			want:              false,
		},
		{
			name:              "workspace multi-module",
			modulesNamesByDir: map[string]string{"/ws/a": "ws/a", "/ws/b": "ws/b"},
			importPath:        "ws/a/pkg",
			want:              true,
		},
		{
			name:              "empty modulesNamesByDir",
			modulesNamesByDir: map[string]string{},
			importPath:        "mymod/pkg",
			want:              false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &packageContext{
				modulesNamesByDir: tt.modulesNamesByDir,
			}
			got := p.isLocalPackage(tt.importPath)
			if got != tt.want {
				t.Errorf("isLocalPackage(%q) = %v, want %v", tt.importPath, got, tt.want)
			}
		})
	}
}

func TestLocalImportersOf(t *testing.T) {
	tests := []struct {
		name              string
		modulesNamesByDir map[string]string
		forward           map[string]map[string]struct{}
		changedModules    []string
		want              []string
	}{
		{
			name:              "local package imports subpackage of changed module",
			modulesNamesByDir: map[string]string{"/repo": "local"},
			forward: map[string]map[string]struct{}{
				"local/a": {"ext/foo/sub": {}},
			},
			changedModules: []string{"ext/foo"},
			want:           []string{"local/a"},
		},
		{
			name:              "multiple local packages import changed module",
			modulesNamesByDir: map[string]string{"/repo": "local"},
			forward: map[string]map[string]struct{}{
				"local/a": {"ext/foo": {}},
				"local/b": {"ext/foo": {}},
			},
			changedModules: []string{"ext/foo"},
			want:           []string{"local/a", "local/b"},
		},
		{
			name:              "no local package imports changed module",
			modulesNamesByDir: map[string]string{"/repo": "local"},
			forward: map[string]map[string]struct{}{
				"local/a": {"ext/bar": {}},
			},
			changedModules: []string{"ext/foo"},
			want:           nil,
		},
		{
			name:              "external package importing changed module is skipped",
			modulesNamesByDir: map[string]string{"/repo": "local"},
			forward: map[string]map[string]struct{}{
				"ext/baz": {"ext/foo": {}},
			},
			changedModules: []string{"ext/foo"},
			want:           nil,
		},
		{
			name:              "multiple changed modules",
			modulesNamesByDir: map[string]string{"/repo": "local"},
			forward: map[string]map[string]struct{}{
				"local/a": {"ext/foo": {}},
				"local/b": {"ext/bar": {}},
			},
			changedModules: []string{"ext/foo", "ext/bar"},
			want:           []string{"local/a", "local/b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &packageContext{
				modulesNamesByDir: tt.modulesNamesByDir,
				forward:           tt.forward,
			}
			got := p.LocalImportersOf(tt.changedModules)
			sort.Strings(got)
			sort.Strings(tt.want)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("LocalImportersOf() (-want, +got)\n%s", diff)
			}
		})
	}
}
