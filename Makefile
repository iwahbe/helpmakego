build: bin/helpmakego

# We don't want to do this, but we don't want to depend on helpmakego to build helpmakego
bin/helpmakego: $(shell find . -name '*.go') go.mod go.sum
	mkdir -p bin
	go build -o $@

.PHONY: lint
lint:
	golangci-lint run
