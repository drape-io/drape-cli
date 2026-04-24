package cidetect

import (
	"fmt"
	"strconv"
	"strings"
)

// parseRunAttempt returns (1, nil) when s is empty (normal for local dev and
// non-GitHub CI), (n, nil) for a valid positive integer, and (1, err) when
// s is set but not a positive integer. Callers that need dedup correctness
// should surface the error; informational callers (BuildURL) can ignore it.
func parseRunAttempt(s string) (int, error) {
	if s == "" {
		return 1, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 1, fmt.Errorf("GITHUB_RUN_ATTEMPT=%q is not a positive integer", s)
	}
	return n, nil
}

func detectGitHub(env EnvFunc) *CIInfo {
	if env("GITHUB_ACTIONS") != "true" {
		return nil
	}

	runID := env("GITHUB_RUN_ID")
	runAttempt, runAttemptErr := parseRunAttempt(env("GITHUB_RUN_ATTEMPT"))

	info := &CIInfo{
		Provider:      "github-actions",
		ProviderName:  "GitHub Actions",
		CommitSHA:     env("GITHUB_SHA"),
		RepoSlug:      env("GITHUB_REPOSITORY"),
		JobID:         env("GITHUB_JOB"),
		BuildNumber:   env("GITHUB_RUN_NUMBER"),
		ProviderRunID: runID,
		RunAttempt:    runAttempt,
		RunAttemptErr: runAttemptErr,
	}

	serverURL := env("GITHUB_SERVER_URL")
	if serverURL == "" {
		serverURL = "https://github.com"
	}
	if info.RepoSlug != "" {
		info.RepoURL = serverURL + "/" + info.RepoSlug
		if runID != "" {
			info.BuildURL = serverURL + "/" + info.RepoSlug + "/actions/runs/" + runID
		}
	}

	// Handle branch/tag from GITHUB_REF
	ref := env("GITHUB_REF")
	switch {
	case strings.HasPrefix(ref, "refs/heads/"):
		info.Branch = strings.TrimPrefix(ref, "refs/heads/")
	case strings.HasPrefix(ref, "refs/tags/"):
		info.Tag = strings.TrimPrefix(ref, "refs/tags/")
	case strings.HasPrefix(ref, "refs/pull/"):
		// PR refs look like "refs/pull/123/merge"
		parts := strings.Split(ref, "/")
		if len(parts) >= 3 {
			info.PRNumber = parts[2]
			info.IsPullRequest = true
		}
	}

	// GITHUB_HEAD_REF is set for pull request events and contains the source branch
	if headRef := env("GITHUB_HEAD_REF"); headRef != "" {
		info.Branch = headRef
		info.IsPullRequest = true
		info.TargetBranch = env("GITHUB_BASE_REF")
	}

	// For PRs, GITHUB_SHA is the merge commit. Use the head SHA if available.
	if info.IsPullRequest {
		if headSHA := env("GITHUB_EVENT_PULL_REQUEST_HEAD_SHA"); headSHA != "" {
			info.CommitSHA = headSHA
		}
	}

	return info
}
