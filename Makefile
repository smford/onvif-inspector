# ==============================================================================
# ONVIF-INSPECTOR Makefile
# ==============================================================================

BINARY_NAME   := onvif-inspector
CMD_DIR       := ./cmd/onvif-inspector
BIN_DIR       := bin
DIST_DIR      := dist

# Build metadata
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "1.0.0")
COMMIT        ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
DATE          ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

# Linker flags to embed version and minimize binary size
LDFLAGS       := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)

GO            ?= go
GOFLAGS       ?= 

.PHONY: all help build install run test test-coverage vet tidy clean cross-compile

## all: Run tests and build binary (default)
all: test build

## help: Display this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':'

## build: Compile onvif-inspector binary
build:
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) $(CMD_DIR)

## install: Install binary to $$GOPATH/bin
install:
	CGO_ENABLED=0 $(GO) install $(GOFLAGS) -ldflags="$(LDFLAGS)" $(CMD_DIR)

## run: Build and run onvif-inspector locally
run: build
	./$(BINARY_NAME)

## test: Run all package unit tests
test:
	$(GO) test -v ./...

## test-coverage: Run tests with code coverage output
test-coverage:
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

## vet: Run go vet on codebase
vet:
	$(GO) vet ./...

## tidy: Run go mod tidy to clean dependencies
tidy:
	$(GO) mod tidy

## clean: Remove compiled binaries and artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -f coverage.out
	rm -rf $(BIN_DIR) $(DIST_DIR)

## cross-compile: Compile static binaries for Linux, macOS, and Windows
cross-compile:
	@mkdir -p $(BIN_DIR)
	@echo "Building for Linux (amd64)..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_DIR)
	@echo "Building for Linux (arm64)..."
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-linux-arm64 $(CMD_DIR)
	@echo "Building for macOS (amd64)..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_DIR)
	@echo "Building for macOS (arm64)..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_DIR)
	@echo "Building for Windows (amd64)..."
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_DIR)
	@echo "Cross-compilation complete. Binaries located in $(BIN_DIR)/"
