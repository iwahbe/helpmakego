# `helpmakego` - A make dependency resolver for Go

Returns a list of files which will be used to compile a go package.

This is specifically designed for integration into makefiles where the timestamps of each source file are used to determine if the target binary needs to be rebuilt.

## Installation

`helpmakego` is designed to be used from directly within a makefile:

```makefile
myprogram: $(go run github.com/iwahbe/helpmakego@v0.1.0 cmd/myprogram)
	go build cmd/myprogram
```

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
$ go run github.com/iwahbe/helpmakego@v0.1.0 cmd/myprogram
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
