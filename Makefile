GO_IMAGE := golang:1.26
GOOS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
GOARCH := $(shell uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')
GO := docker run --rm -v $(CURDIR):/src -w /src -v gh-tickets-gocache:/go -e GOCACHE=/go/cache $(GO_IMAGE)

test:
	$(GO) sh -c 'go vet ./... && go test ./...'

fmt:
	$(GO) gofmt -w .

tidy:
	$(GO) go mod tidy

# Builds the binary for this machine; gh runs it after `gh extension install .`.
build:
	$(GO) env CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -o gh-tickets .

.PHONY: test fmt tidy build
