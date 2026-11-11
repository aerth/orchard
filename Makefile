PREFIX ?= ${HOME}
build:
	go build -v -o bin/ ./...
	go test -c -v -o bin/ ./...
clean:
	rm -rf bin
install:
	install bin/orchard-io ${PREFIX}/bin/orchard-io
