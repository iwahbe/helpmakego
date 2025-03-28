.PHONY: build
build: bin/helpmakego

# We don't want to do this, but we don't want to depend on helpmakego to build helpmakego
bin/helpmakego: $(shell find . -name '*.go') go.mod go.sum
	mkdir -p bin
	go build -o $@

.PHONY: lint
lint:
	golangci-lint run

.PHONY: test
test:
	go test -race -count 1 -v ./...

.PHONY: benchmark
benchmark: bin/helpmakego tmp/helpmakego-main/bin/helpmakego \
		.make/tmp/pulumi \
		.make/tmp/kubernetes

	cd tmp/helpmakego-main && git pull
	make -C tmp/helpmakego-main build

	$(call bench,tmp/pulumi/pkg/cmd/pulumi)
	$(call bench,tmp/kubernetes/cmd/kubectl)

define bench
	hyperfine --warmup 5 \
		--command-name main 'tmp/helpmakego-main/bin/helpmakego $(1)' \
		--command-name current 'bin/helpmakego $(1)'
endef

tmp/helpmakego-main/bin/helpmakego:
	@mkdir -p tmp
	git clone --depth=1 --branch main "https://github.com/iwahbe/helpmakego.git" tmp/helpmakego-main

.make/tmp/pulumi:
	@mkdir -p .make/tmp/
	rm -rf tmp/pulumi
	git clone --depth=1 --branch v3.154.0 "https://github.com/pulumi/pulumi.git" tmp/pulumi
	@touch $@

.make/tmp/kubernetes:
	@mkdir -p .make/tmp/
	rm -rf tmp/kubernetes
	git clone --depth=1 --branch v1.32.2 "https://github.com/kubernetes/kubernetes.git" tmp/kubernetes
	@touch $@

clean:
	rm -rf tmp
	rm -rf bin
	rm -rf .make
