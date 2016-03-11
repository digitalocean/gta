// Copyright 2016 The gta AUTHORS. All rights reserved.
//
// Use of this source code is governed by the Apache 2
// license that can be found in the LICENSE file.

package gta

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Differ implements a single method, used to note code diffs
// and the dirs they occur.
type Differ interface {
	// Diff returns a set of absolute pathed directories
	// that have files that have been modified.
	//
	// upstream is the upstream branch. In many cases this will be "master".
	Diff(upstream string) (map[string]bool, error)
}

// We check to make sure Git implements the Differ interface.
var _ Differ = &Git{}

// Git implements the Differ interface using a git version control method.
type Git struct{}

// Diff returns a set of changed directories.
func (g *Git) Diff(upstream string) (map[string]bool, error) {
	// We get the root of the repository to build our full path.
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil, err
	}
	root := strings.TrimSpace(string(out))

	ref := fmt.Sprintf("origin/%s...HEAD", upstream)
	cmd := exec.Command("git", "diff", ref, "--name-only")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	dirs, err := diffFileDirectories(root, stdout)
	if err != nil {
		return nil, err
	}

	existsDirs := map[string]bool{}
	for dir := range dirs {
		if exists(dir) {
			existsDirs[dir] = false
		}
	}

	return existsDirs, cmd.Wait()
}

func diffFileDirectories(root string, r io.Reader) (map[string]bool, error) {
	dirs := map[string]bool{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		filename := scanner.Text()

		// We build our full absolute file path.
		full, err := filepath.Abs(filepath.Join(root, filename))
		if err != nil {
			return nil, err
		}
		dirs[filepath.Dir(full)] = false
	}

	return dirs, scanner.Err()
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
