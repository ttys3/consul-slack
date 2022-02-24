NAME := consul-slack
TAG := $(shell git describe --always --tags --abbrev=0 | tr -d "[v\r\n]")
COMMIT := $(shell git rev-parse --short HEAD| tr -d "[ \r\n\']")
VERSION :=v$(TAG)-$(COMMIT)
BUILD_TIME := $(shell date +%Y%m%d-%H%M%S)

VERSION_PKG := main
LD_FLAGS := "-w -s -X $(VERSION_PKG).ServiceName=$(NAME) -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).BuildTime=$(BUILD_TIME)"

all: $(NAME)

$(NAME):
	CGO_ENABLED=0 go build -ldflags=$(LD_FLAGS) .

clean:
	-rm -f $(NAME)

podman/build: $(NAME)
	podman build -t $(NAME):$(TAG) -f Dockerfile .

podman/push: podman/build
	podman push $(NAME):$(TAG) docker.io/80x86/$(NAME):$(TAG)

lint:
	golangci-lint run  -v

fmt:
	command -v gofumpt || (WORK=$(shell pwd) && cd /tmp && GO111MODULE=on go get mvdan.cc/gofumpt && cd $(WORK))
	gofumpt -w -s -d .

release: $(NAME)
	@tar czf $(NAME)_linux_amd64.tar.gz $(NAME)

test:
	@go test -i -race
	@go test -v -race -cover $(shell go list ./... | grep -v vendor)
