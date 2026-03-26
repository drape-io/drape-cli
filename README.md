# Drape CLI

Upload test results and coverage reports to [Drape](https://drape.io) from any CI pipeline.

## Installation

### Homebrew

```bash
brew install drape-io/tap/drape
```

### GitHub Releases

Download the latest binary from [GitHub Releases](https://github.com/drape-io/drape-cli/releases):

```bash
# macOS (Apple Silicon)
curl -sL https://github.com/drape-io/drape-cli/releases/latest/download/drape_darwin_arm64.tar.gz | tar xz
chmod +x drape
sudo mv drape /usr/local/bin/

# Linux (amd64)
curl -sL https://github.com/drape-io/drape-cli/releases/latest/download/drape_linux_amd64.tar.gz | tar xz
chmod +x drape
sudo mv drape /usr/local/bin/
```

Note: release archives are named `drape_{version}_{os}_{arch}.tar.gz`. The `/latest/download/` URLs above redirect to the most recent release. For a specific version, use e.g. `drape_1.0.0_linux_amd64.tar.gz`.

### Docker

```bash
docker run --rm ghcr.io/drape-io/drape-cli version
docker run --rm -v $(pwd):/work -w /work ghcr.io/drape-io/drape-cli upload tests "**/*.xml"
```

### Nix

```bash
nix run github:drape-io/drape-cli -- version
nix profile install github:drape-io/drape-cli
```

### GitHub Actions

```yaml
- name: Upload to Drape
  uses: drape-io/upload-action@v1
  with:
    type: tests
    paths: "results.xml"
    api-key: ${{ secrets.DRAPE_API_KEY }}
    org: my-org
    wait: true
```

## Authentication

Set `DRAPE_API_KEY` as an environment variable or pass `--api-key`:

```bash
export DRAPE_API_KEY=drape_tok_xxxx
```

> **Note:** `DRAPE_TOKEN` and `--token` are still accepted for backwards compatibility.

## Commands

### `drape upload tests <glob>`

Upload JUnit XML (or CTRF) test results. Supports glob patterns.

```bash
drape upload tests "./reports/**/*.xml" \
  --org acme \
  --repo my-service \
  --wait \
  --timeout 120
```

When `--wait` is used (default), the CLI waits for server-side processing and prints a summary including quarantine status. Exit code reflects whether non-quarantined failures exist.

### `drape upload coverage <file>`

Upload a coverage report (Cobertura XML, LCOV, or Go coverage profile).

```bash
drape upload coverage coverage.xml \
  --format cobertura \
  --org acme \
  --repo my-service
```

For PR builds, the CLI auto-detects the PR number and target branch from CI environment variables. The server compares your PR's coverage against the base branch and reports:

- **Coverage regression**: previously-covered lines that lost coverage (and weren't modified in the PR)
- **New code coverage**: percentage of new/changed lines that are covered

Example output:

```
Coverage Summary
  Coverage rate: 86.20%
  Files:         42

Coverage Diff (PR #42)
  Base:      85.50%
  Head:      86.20%  (+0.70%)
  New code:  92.00%  (23/25 lines)
  Regressed: 0 lines
  Result:    PASSED
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--format` | (required) | `cobertura`, `lcov`, or `go` |
| `--path-prefix` | | Path prefix for mapping coverage paths to repo paths |
| `--target-branch` | (auto-detected) | Base branch for PR diff comparison |
| `--wait` | `true` | Wait for processing and show diff results |
| `--timeout` | `120` | Max wait time in seconds |

### `drape validate tests <glob>`

Parse and validate JUnit XML files locally without uploading.

```bash
drape validate tests "./reports/**/*.xml"
```

### `drape version`

Print CLI version information.

## Global Flags

| Flag | Env Var | Description |
|------|---------|-------------|
| `--org` | `DRAPE_ORG` | Organization slug |
| `--repo` | `DRAPE_REPO` | Repository name (auto-detected from CI) |
| `--api-key` | `DRAPE_API_KEY` | API key |
| `--api-url` | `DRAPE_API_URL` | API base URL (default: `https://api.drape.io`) |
| `--verbose` | | Enable verbose logging |
| `--dry-run` | | Parse and validate locally, don't upload |

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Test failure (unquarantined failures exist) |
| 2 | Usage error (invalid flags, missing required args) |
| 3 | Upload error (network, auth, or API error) |
| 4 | Timeout (server processing took too long) |
| 5 | Parse error (couldn't parse input files) |
| 6 | Coverage regression (coverage diff check failed) |

## CI Environment Auto-Detection

The CLI automatically detects branch, commit SHA, PR number, and target branch from these CI providers:

- GitHub Actions
- GitLab CI
- CircleCI
- Buildkite
- Jenkins
- Azure Pipelines
- Travis CI
- Bitbucket Pipelines
- Local git (fallback)

All detected values can be overridden with explicit flags.

## CI Integration Examples

### GitHub Actions

```yaml
name: Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Run tests
        run: pytest --junitxml=results.xml --cov --cov-report=xml:coverage.xml
        continue-on-error: true

      - name: Upload test results
        run: drape upload tests results.xml --wait
        env:
          DRAPE_API_KEY: ${{ secrets.DRAPE_API_KEY }}
          DRAPE_ORG: my-org

      - name: Upload coverage
        run: drape upload coverage coverage.xml --format cobertura
        env:
          DRAPE_API_KEY: ${{ secrets.DRAPE_API_KEY }}
          DRAPE_ORG: my-org
```

### GitLab CI

```yaml
test:
  script:
    - pytest --junitxml=results.xml --cov --cov-report=xml:coverage.xml
  after_script:
    - drape upload tests results.xml --wait
    - drape upload coverage coverage.xml --format cobertura
  variables:
    DRAPE_API_KEY: $DRAPE_API_KEY
    DRAPE_ORG: my-org
```

## Coverage Policy

Coverage checks are enforced by default. When a PR uploads coverage, Drape compares against the latest coverage on the target branch and enforces two configurable rules:

1. **No regression** (default: on): Fail if previously-covered lines in unchanged code lose coverage
2. **Minimum new code coverage** (default: off): Require a minimum coverage percentage on new/changed lines

These can be configured per-repository in the Drape dashboard.

**Re-uploads:** Coverage can only be uploaded once per commit SHA. If CI re-runs on the same commit, the upload will be rejected. Push a new commit to re-upload coverage.
