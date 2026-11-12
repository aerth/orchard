PREFIX ?= ${HOME}
build:
	go build -v -o bin/ ./...
	go test -c -v -o bin/ ./...
clean:
	rm -rf bin
install:
	install bin/orchard-io ${PREFIX}/bin/orchard-io
run: build
	bin/orchard-io --log-level info -think medium <<<"Read and understand the main module ./*.go and tell me some things that are missing from README.md that would be helpful" 2> >(jq -c)

