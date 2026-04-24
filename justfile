# justfile for drape-cli development
# https://github.com/casey/just

version := env("VERSION", "dev")
commit  := `git rev-parse --short HEAD 2>/dev/null || echo "none"`
date    := `date -u +%Y-%m-%dT%H:%M:%SZ`
ldflags := "-X main.version=" + version + " -X main.commit=" + commit + " -X main.date=" + date

# Default recipe (runs when you type 'just')
default:
    @just --list

# ============================================================================
# Build & Test
# ============================================================================

# Build the CLI binary with version metadata
build:
    go build -ldflags "{{ldflags}}" -o bin/drape .

# Verify the project compiles (no artifact)
build-check:
    go build -o /dev/null .

# Run tests
test *FLAGS:
    go test ./... -count=1 {{FLAGS}}

# Run tests with verbose output
test-v: (test "-v")

# Run tests with race detector (matches CI)
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

# Run gosec (Go security linter) — pinned to match CI
security-gosec:
    go run github.com/securego/gosec/v2/cmd/gosec@v2.22.4 ./...

# Run govulncheck (Go vulnerability checker, informational)
security-govulncheck:
    go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# Run grype (dependency vulnerability scanner)
security-grype:
    grype dir:. --fail-on high

# Run semgrep
security-semgrep:
    semgrep scan --config auto --error .

# Run all security checks
security: security-gosec security-govulncheck security-grype security-semgrep

# ============================================================================
# Aggregate checks
# ============================================================================

# Quick pre-push check: lint + vet + test
check: lint vet test

# Full CI-equivalent pipeline: race tests + lint + vet + build + security
ci: test-race vet lint build-check security

# ============================================================================
# Release & Cleanup
# ============================================================================

# Build a snapshot release (for testing goreleaser config)
release-snapshot:
    goreleaser release --snapshot --clean

# Cut a release: validate inputs, pre-flight checks, tag, push, watch.
# Usage: just release v0.2.0
release VERSION:
    #!/usr/bin/env bash
    set -euo pipefail
    VERSION="{{VERSION}}"
    if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]]; then
        echo "error: version must be vX.Y.Z or vX.Y.Z-suffix (got: $VERSION)" >&2
        exit 1
    fi
    if git rev-parse "$VERSION" >/dev/null 2>&1; then
        echo "error: tag $VERSION already exists" >&2
        exit 1
    fi
    if [[ -n "$(git status --porcelain)" ]]; then
        echo "error: working tree has uncommitted changes" >&2
        git status --short >&2
        exit 1
    fi
    git fetch origin main --tags --quiet
    local_sha=$(git rev-parse HEAD)
    main_sha=$(git rev-parse origin/main)
    if [[ "$local_sha" != "$main_sha" ]]; then
        echo "error: HEAD ($local_sha) is not on origin/main ($main_sha)" >&2
        echo "       checkout main and pull before releasing" >&2
        exit 1
    fi
    echo "Pre-flight: running tests + vet + lint against $VERSION target..."
    just check
    echo ""
    echo "About to tag origin/main ($main_sha) as $VERSION and push."
    read -p "Continue? [y/N] " -n 1 -r; echo
    [[ ! $REPLY =~ ^[Yy]$ ]] && { echo "aborted"; exit 1; }
    git tag -a "$VERSION" -m "$VERSION"
    git push origin "$VERSION"
    echo ""
    echo "Tag pushed. Watching release workflow (Ctrl-C to detach — release continues)."
    sleep 3
    run_id=$(gh run list --workflow=release.yml --limit 1 --json databaseId --jq '.[0].databaseId')
    gh run watch "$run_id" --exit-status || { echo "release workflow failed"; exit 1; }
    echo ""
    gh release view "$VERSION"

# Tail the most recent release workflow run (for when 'just release' was detached)
release-watch:
    #!/usr/bin/env bash
    run_id=$(gh run list --workflow=release.yml --limit 1 --json databaseId --jq '.[0].databaseId')
    gh run watch "$run_id" --exit-status

# Clean build artifacts
clean:
    rm -rf bin/ dist/

# Run the CLI (pass args after --)
# Example: just run -- upload tests "**/*.xml" --org acme
run *ARGS:
    go run . {{ARGS}}
