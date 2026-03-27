package gta

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDiffGoMod(t *testing.T) {
	tests := []struct {
		name    string
		old     string
		new     string
		want    []ModuleChange
		wantErr bool
	}{
		{
			name: "version bump",
			old:  "module test\ngo 1.21\nrequire foo v1.0.0\n",
			new:  "module test\ngo 1.21\nrequire foo v1.1.0\n",
			want: []ModuleChange{{Path: "foo", OldVersion: "v1.0.0", NewVersion: "v1.1.0"}},
		},
		{
			name: "dependency added",
			old:  "module test\ngo 1.21\n",
			new:  "module test\ngo 1.21\nrequire bar v1.0.0\n",
			want: []ModuleChange{{Path: "bar", NewVersion: "v1.0.0"}},
		},
		{
			name: "dependency removed",
			old:  "module test\ngo 1.21\nrequire baz v1.0.0\n",
			new:  "module test\ngo 1.21\n",
			want: []ModuleChange{{Path: "baz", OldVersion: "v1.0.0"}},
		},
		{
			name: "no change",
			old:  "module test\ngo 1.21\nrequire foo v1.0.0\n",
			new:  "module test\ngo 1.21\nrequire foo v1.0.0\n",
			want: nil,
		},
		{
			name: "multiple changes",
			old:  "module test\ngo 1.21\nrequire (\n\ta v1.0.0\n\tb v1.0.0\n\tc v1.0.0\n)\n",
			new:  "module test\ngo 1.21\nrequire (\n\ta v1.1.0\n\tb v1.2.0\n\td v1.0.0\n)\n",
			want: []ModuleChange{
				{Path: "a", OldVersion: "v1.0.0", NewVersion: "v1.1.0"},
				{Path: "b", OldVersion: "v1.0.0", NewVersion: "v1.2.0"},
				{Path: "d", NewVersion: "v1.0.0"},
				{Path: "c", OldVersion: "v1.0.0"},
			},
		},
		{
			name: "indirect to direct same version",
			old:  "module test\ngo 1.21\nrequire foo v1.0.0 // indirect\n",
			new:  "module test\ngo 1.21\nrequire foo v1.0.0\n",
			want: nil,
		},
		{
			name: "replace added with version",
			old:  "module test\ngo 1.21\nrequire foo v1.0.0\n",
			new:  "module test\ngo 1.21\nrequire foo v1.0.0\nreplace foo v1.0.0 => bar v1.0.0\n",
			want: []ModuleChange{{Path: "foo"}},
		},
		{
			name: "replace version changed",
			old:  "module test\ngo 1.21\nrequire foo v1.0.0\nreplace foo v1.0.0 => bar v1.0.0\n",
			new:  "module test\ngo 1.21\nrequire foo v1.0.0\nreplace foo v1.0.0 => bar v1.1.0\n",
			want: []ModuleChange{{Path: "foo"}},
		},
		{
			name: "replace removed with version",
			old:  "module test\ngo 1.21\nrequire foo v1.0.0\nreplace foo v1.0.0 => bar v1.0.0\n",
			new:  "module test\ngo 1.21\nrequire foo v1.0.0\n",
			want: []ModuleChange{{Path: "foo"}},
		},
		{
			name: "both empty modules",
			old:  "module test\ngo 1.21\n",
			new:  "module test\ngo 1.21\n",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := diffGoMod([]byte(tt.old), []byte(tt.new))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Sort both for deterministic comparison
			sortChanges := func(sl []ModuleChange) {
				sort.Slice(sl, func(i, j int) bool {
					return sl[i].Path < sl[j].Path
				})
			}
			sortChanges(got)
			sortChanges(tt.want)

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("(-want, +got)\n%s", diff)
			}
		})
	}
}

func TestDiffGoSum(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
		want []string
	}{
		{
			name: "version added",
			old:  "",
			new:  "golang.org/x/text v0.4.0 h1:abc\n",
			want: []string{"golang.org/x/text"},
		},
		{
			name: "hash changed",
			old:  "x/text v0.3.0 h1:abc\n",
			new:  "x/text v0.3.0 h1:def\n",
			want: []string{"x/text"},
		},
		{
			name: "version removed",
			old:  "x/text v0.3.0 h1:abc\n",
			new:  "",
			want: []string{"x/text"},
		},
		{
			name: "no change",
			old:  "x/text v0.3.0 h1:abc\n",
			new:  "x/text v0.3.0 h1:abc\n",
			want: nil,
		},
		{
			name: "transitive dep added",
			old:  "a v1.0.0 h1:x\n",
			new:  "a v1.0.0 h1:x\nb v1.0.0 h1:y\n",
			want: []string{"b"},
		},
		{
			name: "multiple changes",
			old:  "a v1.0.0 h1:x\nc v1.0.0 h1:z\n",
			new:  "a v2.0.0 h1:y\nc v2.0.0 h1:w\n",
			want: []string{"a", "c"},
		},
		{
			name: "both empty",
			old:  "",
			new:  "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffGoSum([]byte(tt.old), []byte(tt.new))
			sort.Strings(got)
			sort.Strings(tt.want)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("(-want, +got)\n%s", diff)
			}
		})
	}
}

func TestGoSumModulePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"golang.org/x/text v0.4.0 h1:abc", "golang.org/x/text"},
		{"foo v1.0.0/go.mod h1:abc", "foo"},
		{"", ""},
	}

	for _, tt := range tests {
		got := goSumModulePath(tt.input)
		if got != tt.want {
			t.Errorf("goSumModulePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
