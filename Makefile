VERSION ?= dev-$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)

BINARY  := cloudcode
GOFLAGS := -trimpath

.PHONY: build test clean

build:
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o bin/$(BINARY) ./cmd/cloudcode

test:
	go test ./...

clean:
	rm -rf bin/
