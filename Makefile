VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build test lint clean release-snapshot

build:
	go build -ldflags "$(LDFLAGS)" -o bin/drape .

test:
	go test ./... -v

lint:
	@which golangci-lint > /dev/null 2>&1 || echo "golangci-lint not installed"
	golangci-lint run

clean:
	rm -rf bin/ dist/

release-snapshot:
	goreleaser release --snapshot --clean
