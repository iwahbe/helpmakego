# `helpmakego` - A make dependency resolver for Go

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

## Usage Example

To use `helpmakego` in a Makefile, you can set up your dependencies like this:

```makefile
# Makefile

# Ensure that `helpmakego` is installed at ${HELPMAKEGO} before it is used to resolve targets.
#
# This has the side effect of ensuring that the `bin` directory is present.
HELPMAKEGO_VERSION := v0.1.0
HELPMAKEGO := bin/${HELPMAKEGO_VERSION}/helpmakego
_ := $(shell if ! [ -x ${HELPMAKEGO} ]; then \
	GOBIN=$(shell pwd)/bin/${HELPMAKEGO_VERSION} go install github.com/iwahbe/helpmakego@${HELPMAKEGO_VERSION}; \
	fi \
)

# Define the target and its dependencies

bin/my_tool: $(shell ${HELPMAKEGO} ./cmd/my_tool)
	@echo "Building my_tool..."
	go build -o $@ ./cmd/my_tool

# package.zip may be expensive to build, but it will only be rebuilt when necessary.
package.zip: bin/my-tool
    zip my-tool other-files
```

In this example, `helpmakego .` is used to dynamically generate the list of file
dependencies for `my_target`. This ensures that any changes in the Go module's
dependencies will trigger a rebuild of `my_target`, and that `my_target` will only rebuild
when it needs to.
