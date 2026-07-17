VERSION := $(shell git tag | sort -V | tail -n 1)
NOW := $(shell date +%s)
build:
	go build -v -ldflags '-X main.version=${VERSION} -X main.buildtime=${NOW}' -o ./bin/ ./cmd/...
