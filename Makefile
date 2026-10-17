BINARY  := termino
PKG     := github.com/lesquel/termino
MAIN    := ./cmd/termino
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(PKG)/internal/cli.Version=$(VERSION)

export CGO_ENABLED := 0

.PHONY: all build test vet lint race bench install clean release-snapshot

all: lint vet test build

build: ## Build ./bin/termino
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(MAIN)

test: ## Run all tests
	go test ./...

vet: ## Run go vet
	go vet ./...

lint: ## Fail if any Go file is not gofmt-formatted
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

race: ## Run tests with the race detector (needs cgo)
	CGO_ENABLED=1 go test -race ./...

bench: ## Run latency benchmarks
	go test -run '^$$' -bench . -benchmem ./...

install: ## Install into $(GOBIN) or $(GOPATH)/bin
	go install -trimpath -ldflags "$(LDFLAGS)" $(MAIN)

clean:
	rm -rf bin dist

release-snapshot: ## Local goreleaser build without publishing
	goreleaser release --snapshot --clean
