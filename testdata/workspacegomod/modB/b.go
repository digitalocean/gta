package b

import "example.test/dep"

// B depends on the external dependency, so a version change of
// example.test/dep in modB/go.mod must mark this package.
func B() int {
	return dep.Answer()
}
