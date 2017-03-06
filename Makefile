GCFLAGS ?= -s
VERSION ?= dev

release:
	@go build -gcflags="$(GCFLAGS)"
	@tar czf consul-slack_$(VERSION)_linux_amd64.tar.gz consul-slack
