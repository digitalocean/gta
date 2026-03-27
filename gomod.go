/*
Copyright 2016 The gta AUTHORS. All rights reserved.

Use of this source code is governed by the Apache 2 license that can be found
in the LICENSE file.
*/
package gta

import (
	"fmt"
	"strings"

	"golang.org/x/mod/modfile"
)

// ModuleChange describes a change to a dependency module.
type ModuleChange struct {
	Path       string // module path, e.g. "golang.org/x/text"
	OldVersion string // empty if newly added
	NewVersion string // empty if removed
}

// parseGoMod attempts to parse go.mod data, trying strict Parse first
// (which handles Replace directives) and falling back to ParseLax.
func parseGoMod(name string, data []byte) (*modfile.File, error) {
	f, err := modfile.Parse(name, data, nil)
	if err != nil {
		// Fall back to ParseLax which is more lenient with version strings
		f, err = modfile.ParseLax(name, data, nil)
		if err != nil {
			return nil, err
		}
	}
	return f, nil
}

// diffGoMod compares two go.mod file contents and returns the modules
// whose versions changed, were added, or were removed.
func diffGoMod(oldData, newData []byte) ([]ModuleChange, error) {
	oldFile, err := parseGoMod("old/go.mod", oldData)
	if err != nil {
		return nil, fmt.Errorf("parsing old go.mod: %w", err)
	}
	newFile, err := parseGoMod("new/go.mod", newData)
	if err != nil {
		return nil, fmt.Errorf("parsing new go.mod: %w", err)
	}

	oldReqs := make(map[string]string)
	for _, r := range oldFile.Require {
		oldReqs[r.Mod.Path] = r.Mod.Version
	}

	newReqs := make(map[string]string)
	for _, r := range newFile.Require {
		newReqs[r.Mod.Path] = r.Mod.Version
	}

	var changes []ModuleChange

	// Changed or added
	for path, newVer := range newReqs {
		oldVer, existed := oldReqs[path]
		if !existed {
			changes = append(changes, ModuleChange{Path: path, NewVersion: newVer})
		} else if oldVer != newVer {
			changes = append(changes, ModuleChange{Path: path, OldVersion: oldVer, NewVersion: newVer})
		}
	}

	// Removed
	for path, oldVer := range oldReqs {
		if _, exists := newReqs[path]; !exists {
			changes = append(changes, ModuleChange{Path: path, OldVersion: oldVer})
		}
	}

	// Also diff Replace directives
	oldReplace := make(map[string]string)
	for _, r := range oldFile.Replace {
		key := r.Old.Path + "@" + r.Old.Version
		oldReplace[key] = r.New.Path + "@" + r.New.Version
	}
	newReplace := make(map[string]string)
	for _, r := range newFile.Replace {
		key := r.Old.Path + "@" + r.Old.Version
		newReplace[key] = r.New.Path + "@" + r.New.Version
	}
	for key, newTarget := range newReplace {
		oldTarget, existed := oldReplace[key]
		modPath := strings.SplitN(key, "@", 2)[0]
		if !existed || oldTarget != newTarget {
			changes = append(changes, ModuleChange{Path: modPath})
		}
	}
	for key := range oldReplace {
		if _, exists := newReplace[key]; !exists {
			modPath := strings.SplitN(key, "@", 2)[0]
			changes = append(changes, ModuleChange{Path: modPath})
		}
	}

	return changes, nil
}

// diffGoSum compares two go.sum file contents and returns the module
// paths whose resolved versions changed. This captures transitive
// dependency changes that go.mod diff alone would miss.
func diffGoSum(oldData, newData []byte) []string {
	oldEntries := parseGoSumEntries(oldData)
	newEntries := parseGoSumEntries(newData)

	changed := make(map[string]struct{})

	// New or changed entries
	for key := range newEntries {
		if _, ok := oldEntries[key]; !ok {
			modPath := goSumModulePath(key)
			changed[modPath] = struct{}{}
		}
	}

	// Removed entries
	for key := range oldEntries {
		if _, ok := newEntries[key]; !ok {
			modPath := goSumModulePath(key)
			changed[modPath] = struct{}{}
		}
	}

	var result []string
	for modPath := range changed {
		result = append(result, modPath)
	}
	return result
}

// parseGoSumEntries parses go.sum content into a set of
// "module version hash" lines for comparison.
func parseGoSumEntries(data []byte) map[string]struct{} {
	entries := make(map[string]struct{})
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		entries[line] = struct{}{}
	}
	return entries
}

// goSumModulePath extracts the module path from a go.sum line.
// A go.sum line has the format: "module version hash"
func goSumModulePath(line string) string {
	fields := strings.Fields(line)
	if len(fields) >= 1 {
		return fields[0]
	}
	return line
}
