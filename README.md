# gta

## Overview

`gta` is an application which finds Go packages that have deviated from their upstream
source in git. A typical situation is when a project is using a
[monorepo](https://www.digitalocean.com/blog/taming-your-go-dependencies/).
At build or continuous integration time, you won't have to build every single package
since you will know which packages (and dependencies) have changed.

 ![GTA in Action](gta.gif)

## Installation

```sh
go install github.com/digitalocean/gta/cmd/gta
```

After installation, you will have a `gta` binary in `$GOPATH/bin/`

## Usage

List packages that should be tested since they have deviated from master.

```sh
gta -include "$(go list -f '{{ .Module.Path }}')/"
```

List packages that have deviated from the most recent merge commit.

```sh
gta -include "$(go list -f '{{ .Module.Path }}')/"
```

## What gta does

`gta` builds a list of "dirty" (changed) packages from master, using git. This is useful for determining which
tests to run in larger `monorepo` style repositories.

`gta` works by implementing a various set of interfaces, namely the `Differ` and `Packager` interfaces.

Note: When using this tool, it is common to hit the maximum number of open file descriptors limit set by your OS.
On macOS, this may be a measly 256. Consider raising that maximum to something reasonable with:

```
sudo ulimit -n 4096
```

## Tool Arguments
| Argument          | Description                                                                                                                                                                                                                      | Example                                                                     |
|-------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------|
| `-base`           | sets the base branch for the process. default: `origin/master`                                                                                                                                                                   | `gta -base origin/my-branch`                                                |
| `-include`        | A comma separated list of packages to include.                                                                                                                                                                                   | `gta -include "github.com/myorg/myproject/pkg,github.com/myorg/myproject2"` |
| `-merge`          | A boolean flag to compare against the last merged commit from the base. It cannot be used together with `-h2h` and `-changed-files`.                                                                                             | `gta -merge`                                                                |
| `-json`           | A boolean flag that changes output format to json.                                                                                                                                                                               | `gta -json`                                                                 |
| `-buildable-only` | A boolean flag to look up only the buildable packages between the changes. Those with an at least one `.go` file inside. It cannot be used together with `-json`.                                                                | `gta -buildable-only`                                                       |
| `-changed-files`  | A boolean flag to provide a custom file list of line-breaked paths to check the dependent ones of those instead of using git to detect the changes. Paths must be absolute. It cannot be used together with `-merge` and `-h2h`. | `gta -changed-files changed_files.txt`                                      |
| `-tags`           | A comma separated list of `// +build` tags to consider. This means that gta will filter for files with the input tags in the detected changes.                                                                                   | `gta -tags "linux,debug,test"`                                              |
| `-h2h`            | A boolean flag to compare base and current branch `HEAD` to `HEAD` instead of comparing against the root commit shared with the base branch. It cannot be used together with `-merge` and `changed-files`                        | `gta -h2h`                                                                  |

## License

This application is distributed under the Apache 2 license found in [LICENSE](LICENSE)
