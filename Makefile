build: bin/helpmakego

# We don't want to do this, but we don't want to depend on helpmakego to build helpmakego
bin/helpmakego: $(shell find . -name '*.go') go.mod go.sum
	mkdir -p bin
	go build -o $@

.PHONY: lint
lint:
	golangci-lint run

test:
	go test -race -v ./...

.PHONY: benchmark
benchmark: build tmp/helpmakego-main/bin/helpmakego \
		.make/tmp/pulumi

	$(call bench,tmp/pulumi/pkg/cmd/pulumi)

define bench
	hyperfine --warmup 5 \
		--command-name main 'tmp/helpmakego-main/bin/helpmakego $(1)' \
		--command-name current 'bin/helpmakego $(1)'
endef

tmp/helpmakego-main/bin/helpmakego:
	@mkdir -p tmp
	git clone --depth=1 --branch main "https://github.com/iwahbe/helpmakego.git" tmp/helpmakego-main
	make -C tmp/helpmakego-main build

.make/tmp/pulumi:
	@mkdir -p .make/tmp/
	rm -rf tmp/pulumi
	git clone --depth=1 --branch v3.154.0 "https://github.com/pulumi/pulumi.git" tmp/pulumi
	@touch $@

clean:
	rm -rf tmp
	rm -rf bin
	rm -rf .make
