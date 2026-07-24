VERSION := $(shell git tag | sort -V | tail -n 1)
NOW := $(shell date +%s)
build: *.go */*.go */*/*.go
	go build -v -ldflags '-X main.version=${VERSION} -X main.buildtime=${NOW} -X main.ollamahost=${OLLAMA_HOST} -X main.ollamamodel=${OLLAMA_MODEL} -X main.ollamathink=${OLLAMA_THINK}' -o ./bin/ ./cmd/...
	@test ! -d orchard-chat || echo building orchard-chat TUI
	@test ! -d orchard-chat || make -C orchard-chat
run: build
	./bin/orchard-io -clear <<<'summarize the code in this image [documents/code.png]' 
runclear: build
	./bin/orchard-io -clear
install: build
	mv -v bin/* ~/bin/
	rmdir bin
	@test ! -d orchard-chat || echo installing orchard-chat TUI
	@test ! -d orchard-chat || make -C orchard-chat install


