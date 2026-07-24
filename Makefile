VERSION := $(shell git tag | sort -V | tail -n 1)
NOW := $(shell date +%s)
bin=./bin
WD!=pwd

OCHATDIR=../orchard-chat/
build: ${bin}/orchard-io
${bin}/orchard-io:	*.go */*.go */*/*.go
	go -C ./cmd/orchard-io build -v -ldflags '-X main.version=${VERSION} -X main.buildtime=${NOW} -X main.ollamahost=${OLLAMA_HOST} -X main.ollamamodel=${OLLAMA_MODEL} -X main.ollamathink=${OLLAMA_THINK}' -o ${WD}/bin/ 
	@test ! -d $(OCHATDIR) || echo building $(OCHATDIR) TUI
	@test ! -d $(OCHATDIR) || ${MAKE} -C $(OCHATDIR) 1>/dev/null
run: build
	./bin/orchard-io -clear <<<'summarize the code in this image [documents/code.png]' 
runclear: build
	./bin/orchard-io -clear
install: build
	install -v bin/* ~/bin/
	@test ! -d $(OCHATDIR) || echo installing $(OCHATDIR) TUI
	@test ! -d $(OCHATDIR) || ${MAKE} -C $(OCHATDIR) install 
clean:
	rm -rf bin
	@test ! -d $(OCHATDIR) || ${MAKE} -C $(OCHATDIR) clean
	@echo all clean
cleaninstall:
	rm -rf bin
	@test ! -d $(OCHATDIR) || ${MAKE} -C $(OCHATDIR) clean
	@echo all clean
	${MAKE} ${bin}/orchard-io
	install -v bin/* ~/bin/
	@test ! -d $(OCHATDIR) || echo installing $(OCHATDIR) TUI
	@test ! -d $(OCHATDIR) || ${MAKE} -C $(OCHATDIR) install 
.PHONY += clean cleaninstall



