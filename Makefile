BINARY_NAME := onvif-inspector
MODULE      := github.com/smford/onvif-inspector
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "1.0.0")
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE        ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS     := -s -w \
               -X main.version=$(VERSION) \
               -X main.commit=$(COMMIT) \
               -X main.date=$(DATE)

.PHONY: all help build build-linux-arm64 build-linux-amd64 build-darwin-arm64 build-darwin-amd64 clean test test-coverage lint shellcheck pre-commit sast dast release release-patch release-minor release-major release-tag release-snapshot tag-patch tag-minor tag-major docker-build docker-run run

all: build

help:
	@echo "onvif-inspector development targets:"
	@echo "  build                Build static binary for local architecture"
	@echo "  build-linux-arm64    Cross-compile static binary for linux/arm64"
	@echo "  build-linux-amd64    Cross-compile static binary for linux/amd64"
	@echo "  build-darwin-arm64   Cross-compile static binary for darwin/arm64"
	@echo "  build-darwin-amd64   Cross-compile static binary for darwin/amd64"
	@echo "  test                 Run test suite with race detector (-race)"
	@echo "  test-coverage        Run test suite with code coverage output"
	@echo "  lint                 Verify code formatting (gofmt), go vet, and modules"
	@echo "  shellcheck           Run ShellCheck on shell scripts"
	@echo "  pre-commit           Run pre-commit hooks on all files"
	@echo "  sast                 Run Static Application Security Testing (Semgrep OSS)"
	@echo "  dast                 Run Dynamic Application Security Testing (OWASP ZAP)"
	@echo "  run                  Build and run local onvif-inspector"
	@echo "  release              Initiate GitHub release (default: next patch, or TAG=...)"
	@echo "  release-patch        Calculate next patch version, tag, and push to origin"
	@echo "  release-minor        Calculate next minor version, tag, and push to origin"
	@echo "  release-major        Calculate next major version, tag, and push to origin"
	@echo "  release-snapshot     Build local snapshot release archives via GoReleaser"
	@echo "  docker-build         Build local Docker container image"
	@echo "  docker-run           Run local Docker container with host networking"
	@echo "  clean                Remove build artifacts and temporary binaries"

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) ./cmd/onvif-inspector

build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)-linux-arm64 ./cmd/onvif-inspector

build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)-linux-amd64 ./cmd/onvif-inspector

build-darwin-arm64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)-darwin-arm64 ./cmd/onvif-inspector

build-darwin-amd64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME)-darwin-amd64 ./cmd/onvif-inspector

test:
	go test -v -race -covermode=atomic ./...

test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

lint:
	@echo "Checking formatting (gofmt -s -l)..."
	@unformatted=$$(gofmt -s -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "Unformatted files found:\n$$unformatted" >&2; \
		exit 1; \
	fi
	@echo "Running go vet..."
	go vet ./...
	@echo "Verifying modules..."
	go mod verify
	@if command -v shellcheck >/dev/null 2>&1; then \
		echo "Running shellcheck on shell scripts..."; \
		shellcheck scripts/*.sh; \
	fi

shellcheck:
	@echo "Running shellcheck on shell scripts..."
	shellcheck scripts/*.sh

pre-commit:
	@if command -v pre-commit >/dev/null 2>&1; then \
		pre-commit run --all-files; \
	elif command -v uvx >/dev/null 2>&1; then \
		uvx pre-commit run --all-files; \
	else \
		echo "Error: neither pre-commit nor uvx found" >&2; \
		exit 1; \
	fi

sast:
	uvx semgrep scan --config auto

dast: docker-build
	docker run -d --name onvif-inspector-dast-local --rm -p 8080:8080 -v $$(pwd)/.zap/dast-config.yaml:/etc/onvif-inspector/config.yaml:ro $(BINARY_NAME):latest
	@echo "Waiting for onvif-inspector to start..."
	@sleep 2
	@curl -sf http://127.0.0.1:8080/healthz || (docker stop onvif-inspector-dast-local && exit 1)
	@echo "Running OWASP ZAP Baseline Scan..."
	docker run --rm --network host -v $$(pwd)/.zap:/zap/wrk/:rw ghcr.io/zaproxy/zaproxy:stable zap-baseline.py -t http://localhost:8080 -c rules.tsv -m 3 || true
	docker stop onvif-inspector-dast-local

run: build
	./$(BINARY_NAME)

docker-build:
	docker build -t $(BINARY_NAME):latest .

docker-run:
	docker run --rm -it --network host \
		-v $$(pwd)/config.yaml:/etc/onvif-inspector/config.yaml:ro \
		$(BINARY_NAME):latest

release-snapshot:
	goreleaser release --snapshot --clean

# GitHub Release targets (tags and pushes to origin to trigger GitHub Actions release workflow)
release:
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: Working directory has uncommitted changes. Commit or stash them before releasing." >&2; \
		exit 1; \
	fi
	@if [ -n "$(TAG)" ]; then \
		$(MAKE) release-tag TAG="$(TAG)"; \
	elif [ "$(origin VERSION)" = "command line" ]; then \
		$(MAKE) release-tag TAG="$(VERSION)"; \
	else \
		$(MAKE) release-patch; \
	fi

release-patch:
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: Working directory has uncommitted changes. Commit or stash them before releasing." >&2; \
		exit 1; \
	fi
	@git fetch --tags origin
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	v=$${latest#v}; \
	major=$$(echo $$v | cut -d. -f1); \
	minor=$$(echo $$v | cut -d. -f2); \
	patch=$$(echo $$v | cut -d. -f3); \
	next="v$$major.$$minor.$$((patch + 1))"; \
	$(MAKE) release-tag TAG=$$next

release-minor:
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: Working directory has uncommitted changes. Commit or stash them before releasing." >&2; \
		exit 1; \
	fi
	@git fetch --tags origin
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	v=$${latest#v}; \
	major=$$(echo $$v | cut -d. -f1); \
	minor=$$(echo $$v | cut -d. -f2); \
	next="v$$major.$$((minor + 1)).0"; \
	$(MAKE) release-tag TAG=$$next

release-major:
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: Working directory has uncommitted changes. Commit or stash them before releasing." >&2; \
		exit 1; \
	fi
	@git fetch --tags origin
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	v=$${latest#v}; \
	major=$$(echo $$v | cut -d. -f1); \
	next="v$$((major + 1)).0.0"; \
	$(MAKE) release-tag TAG=$$next

release-tag:
	@if [ -z "$(TAG)" ]; then \
		echo "Error: TAG is required (e.g. make release TAG=v1.0.0)" >&2; \
		exit 1; \
	fi
	@tag="$(TAG)"; \
	case "$$tag" in \
		v*) ;; \
		*) tag="v$$tag" ;; \
	esac; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: Working directory has uncommitted changes. Commit or stash them before releasing." >&2; \
		exit 1; \
	fi; \
	git fetch --tags origin; \
	if git ls-remote --tags --exit-code origin "refs/tags/$$tag" >/dev/null 2>&1; then \
		echo "Error: Tag $$tag already exists on origin." >&2; \
		exit 1; \
	fi; \
	branch=$$(git rev-parse --abbrev-ref HEAD); \
	if [ "$$branch" != "HEAD" ]; then \
		echo "Pushing branch '$$branch' to origin..."; \
		git push origin "$$branch" || exit 1; \
	fi; \
	echo "Initiating release for $$tag..."; \
	if ! git rev-parse -q --verify "refs/tags/$$tag" >/dev/null; then \
		git tag -a "$$tag" -m "Release $$tag" || exit 1; \
	fi; \
	git push origin "$$tag" && \
	echo "Successfully pushed tag $$tag. GitHub Actions release workflow initiated."

# SemVer Local Tagging Helper targets (tags locally only)
tag-patch:
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	v=$${latest#v}; \
	major=$$(echo $$v | cut -d. -f1); \
	minor=$$(echo $$v | cut -d. -f2); \
	patch=$$(echo $$v | cut -d. -f3); \
	next="v$$major.$$minor.$$((patch + 1))"; \
	echo "Tagging $$next locally"; \
	git tag -a $$next -m "Release $$next" && echo "Created tag $$next. Run 'git push origin $$next' or 'make release-patch' to publish."

tag-minor:
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	v=$${latest#v}; \
	major=$$(echo $$v | cut -d. -f1); \
	minor=$$(echo $$v | cut -d. -f2); \
	next="v$$major.$$((minor + 1)).0"; \
	echo "Tagging $$next locally"; \
	git tag -a $$next -m "Release $$next" && echo "Created tag $$next. Run 'git push origin $$next' or 'make release-minor' to publish."

tag-major:
	@latest=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	v=$${latest#v}; \
	major=$$(echo $$v | cut -d. -f1); \
	next="v$$((major + 1)).0.0"; \
	echo "Tagging $$next locally"; \
	git tag -a $$next -m "Release $$next" && echo "Created tag $$next. Run 'git push origin $$next' or 'make release-major' to publish."

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME)-* coverage.out
	rm -rf dist/ bin/
