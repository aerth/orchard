VERSION := $(shell git tag | sort -V | tail -n 1)
NOW := $(shell date +%s)
bin=./bin
WD!=pwd
build: ${bin}/orchard-io
${bin}/orchard-io:	*.go */*.go */*/*.go
	go -C ./cmd/orchard-io build -v -ldflags '-X main.version=${VERSION} -X main.buildtime=${NOW} -X main.ollamahost=${OLLAMA_HOST} -X main.ollamamodel=${OLLAMA_MODEL} -X main.ollamathink=${OLLAMA_THINK}' -o ${WD}/bin/ 
	@test ! -d orchard-chat || echo building orchard-chat TUI
	@test ! -d orchard-chat || ${MAKE} -C orchard-chat 1>/dev/null
run: build
	./bin/orchard-io -clear <<<'summarize the code in this image [documents/code.png]' 
runclear: build
	./bin/orchard-io -clear
install: build
	install -v bin/* ~/bin/
	@test ! -d orchard-chat || echo installing orchard-chat TUI
	@test ! -d orchard-chat || ${MAKE} -C orchard-chat install 
clean:
	rm -rf bin
	@test ! -d orchard-chat || ${MAKE} -C orchard-chat clean
	@echo all clean
cleaninstall:
	rm -rf bin
	@test ! -d orchard-chat || ${MAKE} -C orchard-chat clean
	@echo all clean
	${MAKE} ${bin}/orchard-io
	install -v bin/* ~/bin/
	@test ! -d orchard-chat || echo installing orchard-chat TUI
	@test ! -d orchard-chat || ${MAKE} -C orchard-chat install 
.PHONY += clean cleaninstall



