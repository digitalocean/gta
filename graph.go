/*
Copyright 2016 The gta AUTHORS. All rights reserved.

Use of this source code is governed by the Apache 2 license that can be found
in the LICENSE file.
*/
package gta

// Graph is an adjacency list representation of a graph using maps.
type Graph struct {
	graph map[string]map[string]bool
}

// Traverse is a simple recursive depth first traversal of a directed acyclic graph with no depth limit.
func (g *Graph) Traverse(node string, mark map[string]bool) {
	g.TraverseWithDepth(node, mark, 0)
}

// TraverseWithDepth performs a depth-first traversal of a directed graph,
// stopping after maxDepth hops from the starting node.
// A maxDepth of 0 means unlimited and is equivalent to calling Traverse.
// The starting node itself is always marked regardless of maxDepth.
func (g *Graph) TraverseWithDepth(node string, mark map[string]bool, maxDepth int) {
	g.traverseWithDepth(node, mark, 0, maxDepth)
}

func (g *Graph) traverseWithDepth(node string, mark map[string]bool, currentDepth, maxDepth int) {
	if visited, ok := mark[node]; visited && ok {
		return
	}
	mark[node] = true

	if maxDepth > 0 && currentDepth >= maxDepth {
		return
	}

	if edges, ok := g.graph[node]; ok {
		for edge := range edges {
			g.traverseWithDepth(edge, mark, currentDepth+1, maxDepth)
		}
	}
}
