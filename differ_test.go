package gta

import (
	"bytes"
	"reflect"
	"testing"
)

func Test_diffFileDirectories(t *testing.T) {
	var tests = []struct {
		desc string
		root string
		buf  []byte
		want map[string]bool
	}{
		{
			desc: "single changed file",
			root: "/",
			buf:  []byte("foo/bar.go\n"),
			want: map[string]bool{
				"/foo": false,
			},
		},
		{
			desc: "multiple changed files in same directory (duplicate)",
			root: "/foo",
			buf: []byte(`bar/bar.go
bar/baz.go`),
			want: map[string]bool{
				"/foo/bar": false,
			},
		},
		{
			desc: "multiple changed files in different directories",
			root: "/foo/bar",
			buf: []byte(`baz/bar.go
baz/qux/baz.go`),
			want: map[string]bool{
				"/foo/bar/baz":     false,
				"/foo/bar/baz/qux": false,
			},
		},
		{
			desc: "multiple changed files in different directories, with duplicate directories",
			root: "/",
			buf: []byte(`foo/bar.go
foo/baz.go
bar/foo.go
bar/baz/qux.go
bar/baz/corge.go
bar/baz/qux/corge.go
`),
			want: map[string]bool{
				"/foo":         false,
				"/bar":         false,
				"/bar/baz":     false,
				"/bar/baz/qux": false,
			},
		},
	}

	for _, tt := range tests {
		t.Log(tt.desc)

		got, err := diffFileDirectories(tt.root, bytes.NewReader(tt.buf))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if want, got := tt.want, got; !reflect.DeepEqual(want, got) {
			t.Fatalf("unexpected file directory map:\n- want: %v\n-  got: %v",
				want, got)
		}
	}
}
