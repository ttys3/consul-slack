LDFLAGS ?= -s -w
VERSION ?= dev

release:
	@go build -ldflags="$(LDFLAGS)"
	@tar czf consul-slack_$(VERSION)_linux_amd64.tar.gz consul-slack

test:
	@go test -race -timeout 1m ./...
