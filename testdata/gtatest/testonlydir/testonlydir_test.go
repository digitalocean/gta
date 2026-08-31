package testonlydir_test

import (
	"testing"

	"gta.test/shareddep"
)

// This directory contains only *_test.go files (no production .go).
// When shareddep changes, GTA should mark this package as affected.
func TestUseSharedDep(t *testing.T) {
	_ = shareddep.Value{}
}
