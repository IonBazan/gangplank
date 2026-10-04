VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo unknown)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
CREATED ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

PKG     := github.com/IonBazan/gangplank/cmd
LDFLAGS := -s -w -X $(PKG).version=$(VERSION) -X $(PKG).commit=$(COMMIT) -X $(PKG).created=$(CREATED)

.PHONY: build test cover lint fmt vulncheck snapshot docker clean help

build: ## Build the gangplank binary
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o gangplank .

test: ## Run the tests with the race detector
	go test -race ./...

cover: ## Run the tests and show coverage across packages
	go test -race -coverpkg=./... -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

lint: ## Run golangci-lint
	golangci-lint run ./...

fmt: ## Format the code
	golangci-lint fmt ./...

vulncheck: ## Check dependencies for known vulnerabilities
	go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...

snapshot: ## Build release archives locally without publishing
	goreleaser release --snapshot --clean

docker: ## Build the Docker image for this machine
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg CREATED=$(CREATED) -t gangplank:dev .

clean: ## Remove build output
	rm -rf gangplank dist coverage.out

help: ## Show this help
	@grep -E '^[a-z]+:.*## ' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'
