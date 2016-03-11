# gta: go test auto

## Overview

`gta` is an application which finds Go packages that have deviated from their upstream
source in git. A typical usage situation is when a project is using a 
[monorepo](https://www.digitalocean.com/company/blog/taming-your-go-dependencies/).
At build or continuous integration time, you won't have to build every single package
since you will know which packages (and dependencies) have changed.

 ![GTA in Action](gta.gif)

## Installation

```sh
go get github.com/digitalocean/gta/...
```

After installation, you will have a `gta` binary in `$GOPATH/bin/`

## Usage

List packages that should be tested since they have deviated from master.

```sh
gta -include $(go list ./...)
```

List packages that have deviated from the `development` branch.

```sh
gta -include $(go list ./...) -upstream development
```

## What gta does

`gta` builds a list of "dirty" (changed) packages from master, using git. This is useful for determining which
tests to run in larger `monorepo` style repositories.  

`gta` works by implementing a various set of interfaces, namely the `Differ` and `Packager` interfaces.

Note: When using this tool, it is common to hit the maximum number of open file descriptors limit set by your OS.
On OSX Yosemite, this is a measily 256, consider raising that maximum to something reasonable with:

```
sudo ulimit -n 4096
```

## License

This application is distributed under the Apache 2 license found in [LICENSE](LICENSE)