.PHONY: help build install test test-cli test-engine test-worker test-coverage test-coverage-report test-verbose test-run

help:
	@echo "Entropy Commands"
	@echo "============================"
	@echo ""
	@echo "Build:"
	@echo "  make build                - Build the entropy binary"
	@echo "  make install              - Install entropy to GOPATH/bin"
	@echo ""
	@echo "Testing:"
	@echo "  make test                 - Run all tests in pkg/ (standard Go testing)"
	@echo "  make test-verbose         - Run all tests with verbose output"
	@echo "  make test-coverage        - Run all tests with coverage report"
	@echo "  make test-coverage-report - Generate detailed coverage report"
	@echo ""
	@echo "Module-specific tests:"
	@echo "  make test-cli             - Run CLI tests"
	@echo "  make test-engine          - Run Engine tests"
	@echo "  make test-worker          - Run Worker tests"
	@echo "  make test-config          - Run Config tests"
	@echo "  make test-utils           - Run Utils tests"
	@echo ""
	@echo "Development:"
	@echo "  make test-run TEST=...    - Run specific test by name"
	@echo "  make test-watch           - Run tests on file changes (requires entr)"
	@echo ""

# Build the entropy binary with version info from git
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X github.com/ibrahimkizilarslan/entropy/pkg/cli.Version=$(VERSION)

build:
	@echo "Building entropy $(VERSION)..."
	go build -ldflags "$(LDFLAGS)" -o entropy ./cmd/entropy
	@echo "Built: ./entropy"

# Install entropy to GOPATH/bin
install:
	@echo "Installing entropy $(VERSION)..."
	go install -ldflags "$(LDFLAGS)" ./cmd/entropy
	@echo "Installed to $(shell go env GOPATH)/bin/entropy"

# Run all tests from pkg/ directory (standard Go testing)
test:
	@echo "Running all tests..."
	go test ./pkg/...

# Run all tests with verbose output
test-verbose:
	@echo "Running all tests with verbose output..."
	go test ./pkg/... -v

# Run all tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test ./pkg/... -cover

# Generate detailed coverage report
test-coverage-report:
	@echo "Generating coverage report..."
	go test ./pkg/... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run CLI tests
test-cli:
	@echo "Running CLI tests..."
	go test ./pkg/cli -v

# Run Engine tests
test-engine:
	@echo "Running Engine tests..."
	go test ./pkg/engine -v

# Run Worker tests
test-worker:
	@echo "Running Worker tests..."
	go test ./pkg/worker -v

# Run Config tests
test-config:
	@echo "Running Config tests..."
	go test ./pkg/config -v

# Run Utils tests
test-utils:
	@echo "Running Utils tests..."
	go test ./pkg/utils -v

# Run Integration tests
test-integration:
	@echo "Running Integration tests..."
	go test ./tests/... -v

# Run specific test
test-run:
	@if [ -z "$(TEST)" ]; then \
		echo "Usage: make test-run TEST=TestName"; \
		exit 1; \
	fi
	@echo "Running test: $(TEST)..."
	go test ./pkg/... -v -run $(TEST)

# Run tests on file changes (requires 'entr' tool)
test-watch:
	@command -v entr >/dev/null 2>&1 || { echo "entr not found. Install with: brew install entr or apt-get install entr"; exit 1; }
	@echo "Watching for changes... (Press Ctrl+C to stop)"
	find ./pkg -name "*.go" | entr make test

# Benchmark tests
bench:
	@echo "Running benchmarks..."
	go test ./pkg/... -bench=. -benchmem

# Check test coverage for specific threshold
check-coverage:
	@echo "Checking test coverage (target: 70%)..."
	@go test ./pkg/... -coverprofile=coverage.out -q
	@go tool cover -func=coverage.out | grep total | awk '{print $$3}' | \
		awk 'BEGIN {FS="%"} {coverage=$$1; if(coverage >= 70) {print "✓ Coverage: "coverage"% (PASS)"; exit 0} else {print "✗ Coverage: "coverage"% (FAIL - target: 70%)"; exit 1}}'

# Clean coverage files
clean-coverage:
	@rm -f coverage.out coverage.html
	@echo "Cleaned coverage files"

# Test all packages individually
test-all-packages:
	@echo "Testing all packages..."
	@for pkg in pkg/cli pkg/config pkg/engine pkg/worker pkg/utils pkg/reporter; do \
		echo "\n=== Testing $$pkg ==="; \
		go test ./$$pkg -v || exit 1; \
	done
	@echo "\n✓ All packages tested successfully"

# Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	go test ./pkg/... -race -v

.DEFAULT_GOAL := help
