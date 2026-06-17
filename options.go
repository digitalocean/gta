/*
Copyright 2016 The gta AUTHORS. All rights reserved.

Use of this source code is governed by the Apache 2 license that can be found
in the LICENSE file.
*/
package gta

import "fmt"

// Option is an option function used to modify a GTA.
type Option func(*GTA) error

// SetDiffer sets a differ on a GTA.
func SetDiffer(d Differ) Option {
	return func(g *GTA) error {
		g.differ = d
		return nil
	}
}

// SetPackager sets a packager on a GTA.
func SetPackager(p Packager) Option {
	return func(g *GTA) error {
		g.packager = p
		return nil
	}
}

// SetPrefixes sets a list of prefix to be included
func SetPrefixes(prefixes ...string) Option {
	return func(g *GTA) error {
		g.prefixes = prefixes
		return nil
	}
}

// SetTags sets a list of build tags to consider.
func SetTags(tags ...string) Option {
	return func(g *GTA) error {
		g.tags = tags
		return nil
	}
}

// SetIncludeTransitiveTestDeps sets whether to include test dependencies in the
// dependency graph traversal. When true (the default), packages that are only
// imported by test code are included in the full dependency traversal. When
// false, such test-only dependents are marked but not traversed further.
func SetIncludeTransitiveTestDeps(include bool) Option {
	return func(g *GTA) error {
		g.includeTransitiveTestDeps = include
		return nil
	}
}

// SetRoots sets the root directories for the GTA. When provided, toplevel() is
// not called and the supplied roots are used directly.
func SetRoots(roots ...string) Option {
	return func(g *GTA) error {
		g.roots = roots
		return nil
	}
}

// SetTraversalDepth limits how many hops from a directly changed package the
// reverse-dependency traversal will follow. A depth of 1 marks only the
// immediate dependents of each changed package; a depth of 2 also marks their
// dependents, and so on. The default value of 0 means unlimited — the entire
// reverse-dependency graph is traversed, which is the historical behaviour.
// Negative values are not allowed and will cause New to return an error.
func SetTraversalDepth(depth int) Option {
	return func(g *GTA) error {
		if depth < 0 {
			return fmt.Errorf("traversal depth must be non-negative, got %d", depth)
		}
		g.traversalDepth = depth
		return nil
	}
}
