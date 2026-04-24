// Package cidetect auto-detects CI environment information from environment variables.
package cidetect

// CIInfo holds metadata about the current CI environment.
type CIInfo struct {
	Provider      string // "github-actions", "gitlab-ci", etc.
	ProviderName  string // "GitHub Actions", "GitLab CI", etc.
	BuildURL      string
	BuildNumber   string
	CommitSHA     string
	Branch        string
	TargetBranch  string // for PRs: the base branch
	Tag           string
	RepoSlug      string // owner/repo
	RepoURL       string
	PRNumber      string
	IsPullRequest bool
	JobID         string
	ProviderRunID string // GITHUB_RUN_ID etc. — used as batch natural-key component
	RunAttempt    int    // GITHUB_RUN_ATTEMPT parsed as int; defaults to 1 when unset
	RunAttemptErr error  // non-nil if the attempt env var was set but unparseable
}

// EnvFunc looks up an environment variable by name.
type EnvFunc func(string) string

// detector attempts to detect a CI environment from env vars.
// Returns nil if this CI provider is not detected.
type detector struct {
	name   string
	detect func(env EnvFunc) *CIInfo
}

// detectors is the ordered list of CI provider detectors.
// First match wins.
var detectors = []detector{
	{"github-actions", detectGitHub},
	{"gitlab-ci", detectGitLab},
	{"circleci", detectCircleCI},
	{"buildkite", detectBuildkite},
	{"jenkins", detectJenkins},
	{"azure-pipelines", detectAzure},
	{"travis-ci", detectTravis},
	{"bitbucket-pipelines", detectBitbucket},
}

// Detect auto-detects the CI environment from the given env lookup function.
// Returns nil if no CI environment is detected.
func Detect(env EnvFunc) *CIInfo {
	for _, d := range detectors {
		if info := d.detect(env); info != nil {
			return info
		}
	}
	return nil
}
