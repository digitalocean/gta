package pkg

import "workspace.test/modA/pkg"

func TransitiveUse() string { return pkg.UsesB() }
