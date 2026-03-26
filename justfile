# justfile for drape-cli development
# https://github.com/casey/just

# Default recipe (runs when you type 'just')
default:
    @just --list

# ============================================================================
# Build & Test
# ============================================================================

# Build the CLI binary
build:
    go build -o bin/drape .

# Run tests
test:
    go test ./... -v -count=1

# Run tests with race detector
test-race:
    go test ./... -v -count=1 -race

# Run go vet
vet:
    go vet ./...

# ============================================================================
# Linting
# ============================================================================

# Run golangci-lint
lint:
    golangci-lint run

# Lint GitHub Actions workflows with actionlint
lint-actions:
    actionlint

# Lint GitHub Actions workflows with zizmor
lint-zizmor:
    zizmor --config .github/zizmor.yml --format plain .github/workflows/

# Run all linters
lint-all: lint lint-actions lint-zizmor

# ============================================================================
# Security
# ============================================================================

# Run gosec (Go security linter)
security-gosec:
    go install github.com/securego/gosec/v2/cmd/gosec@latest
    gosec ./...

# Run govulncheck (Go vulnerability checker)
security-govulncheck:
    go install golang.org/x/vuln/cmd/govulncheck@latest
    govulncheck ./...

# Run grype (dependency vulnerability scanner)
security-grype:
    grype dir:. --fail-on high

# Run semgrep
security-semgrep:
    semgrep scan --config auto --error .

# Run all security checks
security: security-gosec security-govulncheck security-grype security-semgrep

# ============================================================================
# All checks
# ============================================================================

# Run all checks (lint + test + vet + security)
check: lint vet test-race security

# Run quick checks (lint + test + vet) - no security scanning
check-quick: lint vet test

# ============================================================================
# Release
# ============================================================================

# Build a snapshot release (for testing goreleaser config)
release-snapshot:
    goreleaser release --snapshot --clean

# Clean build artifacts
clean:
    rm -rf bin/ dist/

# Run the CLI (pass args after --)
# Example: just run upload tests "**/*.xml" --org acme
run *args:
    go run . {{args}}
