build:
	@go build -ldflags="-s -w"

release: build
	@tar czf consul-slack_linux_amd64.tar.gz consul-slack

test:
	@go test -race -timeout 1m ./...
