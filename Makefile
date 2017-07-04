build:
	@go build -ldflags="-s -w"

release: build
	@tar czf consul-slack_linux_amd64.tar.gz consul-slack

test:
	@go test -i -race
	@go test -v -race -cover $(shell go list ./... | grep -v vendor)
