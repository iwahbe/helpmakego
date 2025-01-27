.PHONY: build
build: bin/helpmakego

bin:
	mkdir bin

.PHONY: bin/helpmakego
bin/helpmakego: bin
	go build  -o $@

.PHONY: lint
lint:
	golangci-lint run
