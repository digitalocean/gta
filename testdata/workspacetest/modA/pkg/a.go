package pkg

import "workspace.test/modB/pkg"

func UsesB() string { return pkg.Hello() }
