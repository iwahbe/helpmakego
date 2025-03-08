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
