# jp-go-resilience Makefile
# Standard development commands for the resilience package

# Detect Go binary path and add to PATH for this make session
GOBIN := $(shell go env GOPATH)/bin
export PATH := $(GOBIN):$(PATH)

# Use go run to ensure we use the exact Ginkgo version from go.mod
GINKGO := go run github.com/onsi/ginkgo/v2/ginkgo

# PHONY targets - all targets that don't produce files
.PHONY: help check check-ci test test-race test-coverage fmt lint coverage deps tools clean security

# Default target - show help
help:
	@echo "jp-go-resilience Makefile Commands:"
	@echo ""
	@echo "Core Commands:"
	@echo "  make check              - Run all verification checks (fmt, lint, test)"
	@echo "  make test               - Run tests with coverage"
	@echo "  make test-race          - Run tests with race detection"
	@echo "  make fmt                - Format code with gofumpt"
	@echo "  make lint               - Run golangci-lint (check formatting & security)"
	@echo "  make coverage           - Generate coverage report"
	@echo ""
	@echo "Dependencies:"
	@echo "  make deps               - Download and tidy dependencies"
	@echo "  make tools              - Install required tools"
	@echo ""
	@echo "Other:"
	@echo "  make clean              - Clean build artifacts"
	@echo "  make security           - Run security scan"

#----------------------------------------------
# Primary check command - runs all verification
#----------------------------------------------
check: deps
	@echo ""
	@echo "========================================"
	@echo "Checks - Step 1/3: Formatting"
	@echo "========================================"
	@$(MAKE) fmt
	@echo ""
	@echo "========================================"
	@echo "Checks - Step 2/3: Linting"
	@echo "========================================"
	@$(MAKE) lint
	@echo ""
	@echo "========================================"
	@echo "Checks - Step 3/3: Tests"
	@echo "========================================"
	@$(MAKE) test
	@echo ""
	@echo "✅ All checks passed!"
	@echo ""
	@echo "Note: Race detection available via 'make test-race' (currently has known issues)"

# CI check command - includes coverage for Codecov
check-ci: deps
	@echo ""
	@echo "========================================"
	@echo "CI Checks - Step 1/3: Linting"
	@echo "========================================"
	@$(MAKE) lint
	@echo ""
	@echo "========================================"
	@echo "CI Checks - Step 2/3: Tests with Coverage"
	@echo "========================================"
	@$(MAKE) test-coverage
	@echo ""
	@echo "========================================"
	@echo "CI Checks - Step 3/3: Race Detection"
	@echo "========================================"
	@$(MAKE) test-race
	@echo ""
	@echo "✅ All CI checks passed with coverage!"

#----------------------------------------------
# Core Commands
#----------------------------------------------

# Format code with gofumpt
fmt:
	@echo "Formatting code with gofumpt..."
	@$(GOBIN)/gofumpt -l -w .

# Run linter (checks formatting with gofumpt and runs security scanning with gosec)
lint:
	@echo "Running linter (checks formatting and security)..."
	@$(GOBIN)/golangci-lint run --timeout=5m ./*.go

# Run all tests
test:
	@echo "Running all tests..."
	@$(GINKGO) -r --timeout=3m --succinct

# Run tests with race detection
test-race:
	@echo "Running tests with race detection..."
	@$(GINKGO) -r --race --timeout=10m --succinct

# Run tests with coverage (for CI/CD and Codecov)
test-coverage:
	@echo "Running tests with coverage..."
	@$(GINKGO) -r --timeout=3m --cover --coverprofile=coverage.out --succinct
	@go tool cover -func=coverage.out | grep total | awk '{print "Total coverage: " $$3}'

# Generate coverage report
coverage:
	@echo "Running tests with coverage..."
	@$(GINKGO) -r --timeout=3m --cover --coverprofile=coverage.out --succinct
	@echo "Generating coverage report..."
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
	@go tool cover -func=coverage.out | grep total | awk '{print "Total coverage: " $$3}'

# Run security scan
security:
	@echo "Running security scan..."
	@$(GOBIN)/gosec -terse -fmt text ./...

#----------------------------------------------
# Dependencies and Tools
#----------------------------------------------
# Download and tidy dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@echo "Tidying dependencies..."
	@go mod tidy
	@go mod verify

# Install required tools with pinned versions for reproducibility
tools:
	@echo "Installing required tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8
	@go install mvdan.cc/gofumpt@v0.7.0
	@go install github.com/securego/gosec/v2/cmd/gosec@v2.21.4
	@go install github.com/onsi/ginkgo/v2/ginkgo@v2.25.1
	@echo "✅ All tools installed"

#----------------------------------------------
# Other Commands
#----------------------------------------------
# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -f coverage.out coverage.html cb_coverage.out
	@go clean -cache
