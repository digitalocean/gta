package gta

import (
	"fmt"
	"go/build"

	"golang.org/x/tools/refactor/importgraph"
)

// GraphError is a collection of errors from attempting to build the
// dependent graph.
type GraphError struct {
	Errors map[string]error
}

// Error implements the error interface for GraphError.
func (g *GraphError) Error() string {
	return fmt.Sprintf("errors while generating import graph: %v", g.Errors)
}

// Packager interface defines a set of means to access golang build Package information.
type Packager interface {
	// Get a go package from directory.
	PackageFromDir(string) (*build.Package, error)
	// Get a go package from import path.
	PackageFromImport(string) (*build.Package, error)
	// DependentGraph returns the DependentGraph for the current
	// Golang workspace as defined by their import paths.
	DependentGraph() (*Graph, error)
}

// verify DefaultPackager implements the the Packager interface
var _ Packager = DefaultPackager

// DefaultPackager is the default instance of PackageContext.
var DefaultPackager = &PackageContext{
	ctx: &build.Default,
}

// PackageContext implements the Packager interface using build contexts.
type PackageContext struct {
	ctx *build.Context
}

// PackageFromDir returns a build package from a directory as a string.
func (p *PackageContext) PackageFromDir(dir string) (*build.Package, error) {
	return p.ctx.ImportDir(dir, build.ImportComment)
}

// PackageFromImport returns a build package from an import path as a string.
func (p *PackageContext) PackageFromImport(importPath string) (*build.Package, error) {
	return p.ctx.Import(importPath, ".", build.ImportComment)
}

// DependentGraph returns a dependent graph based on the current golang workspace.
func (p *PackageContext) DependentGraph() (*Graph, error) {
	_, graph, errs := importgraph.Build(p.ctx)
	if len(errs) != 0 {
		return nil, &GraphError{Errors: errs}
	}

	return &Graph{graph: graph}, nil
}
