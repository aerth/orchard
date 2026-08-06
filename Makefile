VERSION := $(shell git tag | sort -V | tail -n 1)
NOW := $(shell date +%s)
WD!=pwd
bin=${WD}/bin
OCHATDIR=../orchard-chat/
build: ${bin}/orchard-io
${bin}/orchard-io:	*.go */*.go */*/*.go */*/*/*.go
	@echo Building orchard-io
	go -C ./cmd/orchard-io build -v -ldflags '-X main.version=${VERSION} -X main.buildtime=${NOW} -X main.ollamahost=${OLLAMA_HOST} -X main.ollamamodel=${OLLAMA_MODEL} -X main.ollamathink=${OLLAMA_THINK}' -o $@
	@test ! -d $(OCHATDIR) || echo "Building Orchard TUI ($(OCHATDIR))"
	@test ! -d $(OCHATDIR) || ${MAKE} -C $(OCHATDIR) 1>/dev/null
run: build
	./bin/orchard-io -clear <<<'summarize the code in this image [documents/code.png]' 
runclear: build
	./bin/orchard-io -clear
install: build
	install -v bin/* ~/bin/
	@test ! -d $(OCHATDIR) || echo installing $(OCHATDIR) TUI
	@test ! -d $(OCHATDIR) || ${MAKE} -C $(OCHATDIR) clean install 
clean:
	rm -rf bin
	@test ! -d $(OCHATDIR) || ${MAKE} -C $(OCHATDIR) clean
	@echo all clean
cleaninstall:
	rm -rf bin
	@test ! -d $(OCHATDIR) || ${MAKE} -C $(OCHATDIR) clean
	@echo all clean
	${MAKE} ${bin}/orchard-io
	test -x ${bin}/orchard-io
	install -v ${bin}/orchard-io ~/bin/
	@test ! -d $(OCHATDIR) || echo installing $(OCHATDIR) TUI
	@test ! -d $(OCHATDIR) || ${MAKE} -C $(OCHATDIR) install 
push:
	git push
	git -C ../orchard-chat push
	git -C ../otools push
.PHONY += clean cleaninstall push

complexity:
	for i in ./pkg/*; do echo$ $i; scc $$i; done

test:
	go test -v -short -count=0 ./...  |& grep -E '^ok|^FAIL|\?'


