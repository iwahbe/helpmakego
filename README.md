# `helpmakego` - A make dependency resolver for Go

Returns a list of files which will be used to compile a go package.

This is specifically designed for integration into Makefiles where the timestamps of each source file are used to determine if the target binary needs to be rebuilt.

## Installation

`helpmakego` is designed to be used from directly within a Makefile:

```makefile
myprogram: $(go tool github.com/iwahbe/helpmakego cmd/myprogram)
	go build cmd/myprogram
```

The easiest way to use `helpmakego` is with `go tool`[^1]:

``` sh
# Run once
go get -tool github.com/iwahbe/helpmakego
```

You can also use `go install github.com/iwahbe/helpmakego@latest` to install globally and
then invoke `helpmakego` directly.

## CLI usage

```text
Usage:
  helpmakego [path-to-package] [--test] [flags]

Flags:
  -h, --help   help for helpmakego
      --test   include test files in the dependency analysis
```

The output by default is a list of space-separated files. If any of these files are edited, the target program will need to be rebuilt.

### Example

```shell
$ go tool github.com/iwahbe/helpmakego cmd/myprogram
cmd/myprogram/main.go go.mod go.sum
```

### Restricting output with `--changed-since <ref>`

`--changed-since <ref>` restricts the output to only the parts of the dependency
set that *could have changed* since `<ref>`. `<ref>` is any revision your version
control system understands, for example `--changed-since origin/master`,
`--changed-since HEAD~2`, a tag, or a commit hash.

#### What counts as changed

A **file** is changed if it differs between `<ref>` and your current working tree.
This is the *net* difference and includes:

- files changed by commits between `<ref>` and your working tree,
- uncommitted changes, both staged and unstaged,
- untracked files.

Because it is the net difference, a file that was edited in an intervening commit
but then reverted back to its `<ref>` contents does *not* count as changed.

#### Changes propagate to importers

A change to a package can change the compiled output of every package that
imports it, so `helpmakego` emits a changed package **and every package that
transitively imports it**.

Given `A <- B <- main` (`B` imports `A`, `main` imports `B`):

- editing a file in `A` emits `A`, `B`, and `main`,
- editing a file only in `main` emits just `main`.

#### Module changes

A change to `go.mod` is handled specially. Bumping a dependency's version, or
adding, changing, or removing a `replace`, can change the compiled output of
every package that imports the affected dependency, even when no local source
file of those packages changed.

`helpmakego` reads `go.mod` as it was at `<ref>` and compares it to the current
`go.mod`. For every required module whose effective version or `replace` differs,
every package that transitively imports that module is treated as changed.

#### Local `replace` directives

A `replace` that points at a local directory is tracked by its files rather than
its version, so a `go.mod` version change for a locally replaced module is
ignored — only the files in the replacement directory decide whether it changed.

- When the directory is in the **same repository** as the entry module, its files
  are already part of the repository's change set, so nothing special is needed.
- When the directory is in a **separate repository**, that repository is diffed on
  its own. The starting revision comes from the version the requiring `go.mod`
  pinned at `<ref>` (every `replace` still carries a `require`): a released version
  maps to its tag, and a pseudo-version maps to the commit it encodes. Any file
  that changed in the replacement repository between that revision and its current
  working tree marks the corresponding package as changed, and the change
  propagates to importers as usual.

If a separate replacement repository cannot be diffed precisely — it is not a
recognized repository, or the pinned revision cannot be resolved — every package
under it is treated as changed.

## How it Works

`helpmakego` is a tool designed to resolve dependencies for Go projects, making it easier
to integrate with Makefiles. It traverses the Go module's directory structure, identifying
all files that are part of the module, including optional test files if specified.

`helpmakego` aims to provide as fine grain a dependency set as possible. It respects:

- `go.mod` (including local `replace` directives) and `go.sum`
- `go.work` (including local `replace` directives) and `go.work.sum`
- `go:embed` directives

Like the `go build` tool itself, `helpmakego` only considers packages that are actually
referenced.

[^1]: Added in Go 1.24: https://pkg.go.dev/cmd/go#hdr-Run_specified_go_tool
