/*
Copyright 2016 The gta AUTHORS. All rights reserved.

Use of this source code is governed by the Apache 2 license that can be found
in the LICENSE file.
*/
package gta

import (
	"testing"

	"github.com/google/go-cmp/cmp"
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
		if diff := cmp.Diff(tt.want, got); diff != "" {
			t.Errorf("(-want, +got)\n%s", diff)
		}
	}
}

func TestGraphTraversalWithDepth(t *testing.T) {
	// A depends on B depends on C depends on D
	chainGraph := &Graph{
		graph: map[string]map[string]bool{
			"D": {"C": true},
			"C": {"B": true},
			"B": {"A": true},
		},
	}

	tests := []struct {
		comment  string
		graph    *Graph
		start    string
		maxDepth int
		want     map[string]bool
	}{
		{
			comment:  "depth 0 (unlimited) traverses full chain from D",
			graph:    chainGraph,
			start:    "D",
			maxDepth: 0,
			want:     map[string]bool{"D": true, "C": true, "B": true, "A": true},
		},
		{
			comment:  "depth 1 marks only the start node and its direct dependents",
			graph:    chainGraph,
			start:    "D",
			maxDepth: 1,
			want:     map[string]bool{"D": true, "C": true},
		},
		{
			comment:  "depth 2 marks the start node, direct dependents, and their dependents",
			graph:    chainGraph,
			start:    "D",
			maxDepth: 2,
			want:     map[string]bool{"D": true, "C": true, "B": true},
		},
		{
			comment:  "depth 1 from a node with no dependents marks only the start node",
			graph:    chainGraph,
			start:    "A",
			maxDepth: 1,
			want:     map[string]bool{"A": true},
		},
		{
			comment: "depth 1 from a branching node marks start and immediate dependents only",
			graph: &Graph{
				graph: map[string]map[string]bool{
					"E": {"C": true, "D": true},
					"C": {"B": true},
					"D": {"B": true},
					"B": {"A": true},
				},
			},
			start:    "E",
			maxDepth: 1,
			want:     map[string]bool{"E": true, "C": true, "D": true},
		},
		{
			comment:  "TraverseWithDepth with depth 0 is identical to Traverse",
			graph:    chainGraph,
			start:    "C",
			maxDepth: 0,
			want:     map[string]bool{"C": true, "B": true, "A": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.comment, func(t *testing.T) {
			got := map[string]bool{}
			tt.graph.TraverseWithDepth(tt.start, got, tt.maxDepth)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("(-want, +got)\n%s", diff)
			}
		})
	}
}
