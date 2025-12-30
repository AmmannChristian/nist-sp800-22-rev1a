# Makefile for nist-sp800-22-rev1a

.PHONY: all proto build build-arm64 run clean test test-ci tests test-cover test-race cover cover-html cover-threshold perpkg-threshold coverage-ci coverage deploy deps dev fmt fmt-fix lint staticcheck gosec govulncheck vet tools tools-update help docker-build bench bench-all bench-compare bench-baseline

# ========================================
# Variables
# ========================================
BINARY_NAME=nist-sp800-22-rev1a
PROTO_DIR=api/nist/v1
PB_DIR=pkg/pb
BUILD_DIR=build
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

GOTESTFLAGS ?= -count=1 -timeout=2m
RACE_TESTFLAGS ?= -count=1 -timeout=3m
UNIT_PKGS ?= ./internal/... ./cmd/... 
UNIT_SHUFFLE ?= on
RACE_SHUFFLE ?= on
COVER_SHUFFLE ?= off
JUNIT_FILE ?=
COVERAGE_MIN ?= 90
COVERMODE ?= atomic
COVERPROFILE ?= $(BUILD_DIR)/coverage.out

DEV_TOOLS=\
	github.com/golangci/golangci-lint/cmd/golangci-lint \
	honnef.co/go/tools/cmd/staticcheck \
	golang.org/x/vuln/cmd/govulncheck \
	golang.org/x/tools/cmd/goimports \
	mvdan.cc/gofumpt \
	github.com/securego/gosec/v2/cmd/gosec \
	google.golang.org/protobuf/cmd/protoc-gen-go \
	google.golang.org/grpc/cmd/protoc-gen-go-grpc

FMT_FIND := find . -type f -name '*.go' -not -path './$(PB_DIR)/*' -not -path './$(BUILD_DIR)/*' -not -path './.cache/*'

# Auto-add GOPATH/bin to PATH for protoc plugins
GOBIN := $(shell go env GOPATH)/bin
export PATH := $(GOBIN):$(PATH)

# ========================================
# Default target
# ========================================
all: proto build

# ========================================
# Generate protobuf code
# ========================================
proto:
	@set -eu; \
	echo "Generating protobuf code..."; \
	mkdir -p $(PB_DIR); \
	protoc \
	  -I $(PROTO_DIR) \
	  --go_out=$(PB_DIR) --go_opt=paths=source_relative \
	  --go-grpc_out=$(PB_DIR) --go-grpc_opt=paths=source_relative \
	  $(PROTO_DIR)/nist_sp800_22.proto; \
	echo "Protobuf generation complete"; \
	find $(PB_DIR) -maxdepth 5 -type f -name '*.pb.go' -print

# ========================================
# Build for local development
# ========================================
build: proto
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/server
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# ========================================
# Build for ARM64 (e.g., Raspberry Pi)
# ========================================
build-arm64: proto
	@echo "Building for ARM64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $(BUILD_DIR)/$(BINARY_NAME)-arm64 ./cmd/server
	@echo "ARM64 build complete: $(BUILD_DIR)/$(BINARY_NAME)-arm64"

# ========================================
# Run locally
# ========================================
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BUILD_DIR)/$(BINARY_NAME)

# ========================================
# Development mode (no build, direct run)
# ========================================
dev: proto
	@echo "Starting development mode..."
	go run ./cmd/server

# ========================================
# Clean build artifacts
# ========================================
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)/
	rm -f $(PB_DIR)/*.pb.go
	@echo "Clean complete"

# ========================================
# Run tests
# ========================================
test:
	@echo "Running tests..."
	go test $(GOTESTFLAGS) -shuffle=$(UNIT_SHUFFLE) ./...

# Deterministic list of packages used in CI; keeps tests/coverage aligned
test-ci:
	@echo "Running CI unit tests (packages: $(UNIT_PKGS))..."
	@echo "Shuffle: $(UNIT_SHUFFLE)"
	@if [ -n "$(JUNIT_FILE)" ] && command -v gotestsum >/dev/null; then \
		mkdir -p $(dir $(JUNIT_FILE)); \
		gotestsum --junitfile $(JUNIT_FILE) --format testname -- \
			$(GOTESTFLAGS) -shuffle=$(UNIT_SHUFFLE) $(UNIT_PKGS); \
	else \
		go test $(GOTESTFLAGS) -shuffle=$(UNIT_SHUFFLE) $(UNIT_PKGS); \
	fi

# Alias for convenience (e.g., `make tests`)
tests: test

# Run tests with coverage reporting
test-cover:
	@$(MAKE) coverage

# Run tests with the race detector enabled
test-race:
	@echo "Running tests with race detector..."
	go test $(RACE_TESTFLAGS) -race -shuffle=$(RACE_SHUFFLE) $(UNIT_PKGS)

cover:
	@$(MAKE) coverage-ci

cover-html:
	@go tool cover -html=$(COVERPROFILE) -o $(BUILD_DIR)/coverage.html

cover-threshold:
	@echo "Checking total coverage ≥ $(COVERAGE_MIN)%..."
	@test -f $(COVERPROFILE) || { echo "Coverage profile $(COVERPROFILE) not found; run 'make coverage-ci' first."; exit 1; }
	@total=$$(go tool cover -func=$(COVERPROFILE) | awk '/^total:/ {print $$3}'); \
	awk -v cov=$$total -v min=$(COVERAGE_MIN) 'BEGIN { cov+=0; if (cov < min) { printf "Coverage %.2f%% < %.0f%%\n", cov, min; exit 1 } else { printf "Coverage %.2f%% ≥ %.0f%%\n", cov, min } }'

perpkg-threshold:
	@echo "Checking per-package coverage ≥ 90%..."
	@test -f $(COVERPROFILE) || { echo "Coverage profile $(COVERPROFILE) not found; run 'make coverage-ci' first."; exit 1; }
	@go tool cover -func=$(COVERPROFILE) | awk '/\.go:/ { next } /^total:/ { next } { gsub(/%/,"",$$3); if ($$3+0 < 90.0) { bad=1; printf "  %s -> %.2f%%\n", $$1, $$3 } } END { exit(bad) }'

staticcheck:
	@echo "Running staticcheck..."
	GOCACHE=$(abspath $(BUILD_DIR)/gocache) \
	XDG_CACHE_HOME=$(abspath $(BUILD_DIR)/.cache) \
	staticcheck $(UNIT_PKGS)

gosec:
	@echo "Running gosec..."
	mkdir -p tools/ci
	GOCACHE=$(BUILD_DIR)/gocache gosec -exclude-dir=internal/service -exclude-dir=.cache -exclude-generated -conf tools/ci/.gosec.json ./...

govulncheck:
	@echo "Running govulncheck..."
	govulncheck ./...

coverage-ci:
	@echo "Generating deterministic coverage profile..."
	@mkdir -p $(BUILD_DIR)
	GOCACHE=$(abspath $(BUILD_DIR)/gocache) XDG_CACHE_HOME=$(abspath $(BUILD_DIR)/.cache) go test $(GOTESTFLAGS) -shuffle=$(COVER_SHUFFLE) -covermode=$(COVERMODE) -coverprofile=$(COVERPROFILE) $(UNIT_PKGS)
	@GOCACHE=$(abspath $(BUILD_DIR)/gocache) go tool cover -func=$(COVERPROFILE) | tail -n 1

coverage:
	@$(MAKE) coverage-ci
	@$(MAKE) cover-threshold

# ========================================
# Benchmarking
# ========================================
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem -benchtime=10s ./internal/nist/

bench-all:
	@echo "Running all benchmarks with 10 iterations..."
	go test -bench=. -benchmem -benchtime=10s -count=10 ./internal/nist/ | tee $(BUILD_DIR)/bench-current.txt

bench-compare:
	@echo "Comparing benchmarks with baseline..."
	@test -f $(BUILD_DIR)/bench-baseline.txt || { echo "No baseline found. Run 'make bench-baseline' first."; exit 1; }
	@echo "Running current benchmarks..."
	@go test -bench=. -benchmem -count=5 ./internal/nist/ | tee $(BUILD_DIR)/bench-current.txt
	@if command -v benchstat >/dev/null 2>&1; then \
		echo ""; \
		echo "Benchmark comparison:"; \
		benchstat $(BUILD_DIR)/bench-baseline.txt $(BUILD_DIR)/bench-current.txt; \
	else \
		echo ""; \
		echo "Install benchstat for statistical comparison:"; \
		echo "  go install golang.org/x/perf/cmd/benchstat@latest"; \
	fi

bench-baseline:
	@echo "Capturing baseline benchmarks..."
	@mkdir -p $(BUILD_DIR)
	go test -bench=. -benchmem -count=10 ./internal/nist/ | tee $(BUILD_DIR)/bench-baseline.txt
	@echo ""
	@echo "Baseline saved to $(BUILD_DIR)/bench-baseline.txt"

# ========================================
# Install dependencies
# ========================================
deps:
	@echo "Installing Go dependencies..."
	go mod download
	@echo "Tidying modules..."
	go mod tidy
	@$(MAKE) tools
	@echo "Dependencies installed"
	@echo ""
	@echo "Note: You also need 'protoc' compiler:"
	@echo "  Linux: sudo apt install protobuf-compiler"
	@echo "  macOS: brew install protobuf"

tools:
	@echo "Installing developer tools..."
	@set -e; for tool in $(DEV_TOOLS); do \
		echo "  $$tool"; \
		GOFLAGS='-mod=readonly -tags=tools' go install $$tool; \
	done

tools-update:
	@echo "Checking for newer tool versions..."
	@for tool in $(DEV_TOOLS); do \
		name=$${tool%@*}; \
		current=$${tool#*@}; \
		module=$$(echo $$name | sed -E 's|/cmd/.*$$||'); \
		[ -n "$$module" ] || module=$$name; \
		latest=$$(go list -m -versions $$module 2>/dev/null | tr ' ' '\n' | tail -n 1); \
		printf "%s: current=%s latest=%s\n" "$$name" "$$current" "$$latest"; \
	done

# ========================================
# Docker build
# ========================================
docker-build:
	@echo "Building Docker image..."
	docker build -t $(BINARY_NAME):$(VERSION) .
	@echo "Docker build complete"

# ========================================
# Format code
# ========================================
fmt: fmt-fix

fmt-fix:
	@echo "Running gofumpt..."
	@$(FMT_FIND) -print0 | xargs -0 gofumpt -w
	@echo "Running gofmt -s..."
	@$(FMT_FIND) -print0 | xargs -0 gofmt -s -w
	@echo "Running goimports..."
	@$(FMT_FIND) -print0 | xargs -0 goimports -w
	@echo "Formatting complete"

# ========================================
# Lint code
# ========================================
lint:
	@echo "Running linters..."
	@$(MAKE) staticcheck
	@$(MAKE) vet

vet:
	@echo "Running go vet..."
	GOCACHE=$(abspath $(BUILD_DIR)/gocache) XDG_CACHE_HOME=$(abspath $(BUILD_DIR)/.cache) go vet ./...

# ========================================
# Help
# ========================================
help:
	@echo "Available targets:"
	@echo "  make proto           - Generate protobuf code (outputs to $(PB_DIR))"
	@echo "  make build           - Build for local development"
	@echo "  make build-arm64     - Build for ARM64"
	@echo "  make run             - Build and run locally"
	@echo "  make dev             - Run without building (development)"
	@echo "  make clean           - Remove build artifacts"
	@echo "  make test            - Run tests"
	@echo "  make tests           - Alias for 'make test'"
	@echo "  make test-cover      - Run tests with coverage reporting"
	@echo "  make test-race       - Run tests with the race detector"
	@echo "  make cover           - Generate coverage for internal packages"
	@echo "  make cover-html      - Render coverage HTML report"
	@echo "  make cover-threshold - Fail if coverage falls below $(COVERAGE_MIN)%"
	@echo "  make deps            - Install dependencies"
	@echo "  make fmt             - Format code"
	@echo "  make fmt-fix         - Apply gofumpt/gofmt/goimports"
	@echo "  make lint            - Run linters"
	@echo "  make vet             - Run go vet"
	@echo "  make tools           - Install pinned developer tools"
	@echo "  make bench           - Run benchmarks"
	@echo "  make bench-all       - Run benchmarks with 10 iterations"
	@echo "  make bench-baseline  - Capture baseline benchmarks"
	@echo "  make bench-compare   - Compare with baseline (requires benchstat)"
