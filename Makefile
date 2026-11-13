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
test:
	#go test -json -v -short -failfast ./... | jq -c
	go test -v -short -failfast ./...
docker:
	test ! -d tmp/otools || git -C tmp/otools pull origin master
	test -d tmp/otools/.git || git clone https://github.com/aerth/orchard-tools tmp/otools
	docker build --network=host -t orchard .
	@echo built container
	@test -f ~/.config/orchard/env || echo create ~/.config/orchard/env file to run quick docker test
	@test ! -f ~/.config/orchard/env || docker run --network=host \
		-v /etc/ssl/certs/ca-certificates.crt:/etc/ssl/certs/ca-certificates.crt:ro \
		--hostname "orchard-os" \
		--env-file ~/.config/orchard/env \
		-i \
		orchard <<<"Tell me about this machine, environment. What programs are installed, etc"
