VERSION := $(shell git tag | sort -V | tail -n 1)
NOW := $(shell date +%s)
build: bin/
bin/: *.go */*.go */*/*.go
	go build -v -ldflags '-X main.version=${VERSION} -X main.buildtime=${NOW} -X main.ollamahost=${OLLAMA_HOST} -X main.ollamamodel=${OLLAMA_MODEL} -X main.ollamathink=${OLLAMA_THINK}' -o ./bin/ ./cmd/...
run: build
	./bin/orchard-io <<<'summarize the code in this image [documents/code.png]' 
runclear: build
	./bin/orchard-io -clear
install: bin/
	mv -nv bin/* ~/bin/


