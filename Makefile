build: bin/helpmakego

bin:
	mkdir bin

bin/helpmakego: bin
	go build  -o $@

lint:
	golangci-lint run
