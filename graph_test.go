package gta

import (
	"reflect"
	"testing"
)

func TestGraphTraversal(t *testing.T) {
	tests := []struct {
		graph   *Graph
		start   string
		want    map[string]bool
		comment string
	}{
		{
			comment: "A depends on B depends on C, C is dirty, so we expect all of them to be marked",
			graph: &Graph{
				graph: map[string]map[string]bool{
					"C": map[string]bool{
						"B": true,
					},
					"B": map[string]bool{
						"A": true,
					},
				},
			},
			start: "C",
			want: map[string]bool{
				"A": true,
				"B": true,
				"C": true,
			},
		},
		{
			comment: "A depends on B depends on C, B is dirty, so we expect just A and B, and NOT C to be marked",
			graph: &Graph{
				graph: map[string]map[string]bool{
					"C": map[string]bool{
						"B": true,
					},
					"B": map[string]bool{
						"A": true,
					},
				},
			},
			start: "B",
			want: map[string]bool{
				"A": true,
				"B": true,
			},
		},
		{
			comment: "A depends on B depends on C depends on D, E depends on C, C and E dirty, so we expect all of them to be marked but D",
			graph: &Graph{
				graph: map[string]map[string]bool{
					"D": map[string]bool{
						"C": true,
					},
					"C": map[string]bool{
						"B": true,
						"E": true,
					},
					"B": map[string]bool{
						"A": true,
					},
				},
			},
			start: "C",
			want: map[string]bool{
				"A": true,
				"B": true,
				"C": true,
				"E": true,
			},
		},
	}

	for _, tt := range tests {
		t.Log(tt.comment)
		got := map[string]bool{}
		tt.graph.Traverse(tt.start, got)
		if !reflect.DeepEqual(tt.want, got) {
			t.Error("expected want and got to be equal")
			t.Errorf("want: %v", tt.want)
			t.Errorf(" got: %v", got)
		}
	}
}
