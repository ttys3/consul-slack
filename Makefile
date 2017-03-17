LDFLAGS ?= -s -w
VERSION ?= dev

release:
	@go build -ldflags="$(LDFLAGS)" -o consul-slack
	@tar czf consul-slack_$(VERSION)_linux_amd64.tar.gz consul-slack
	@rm consul-slack

test:
	@go test -race -timeout 1m ./...
