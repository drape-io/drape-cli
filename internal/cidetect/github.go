package cidetect

import "strings"

func detectGitHub(env EnvFunc) *CIInfo {
	if env("GITHUB_ACTIONS") != "true" {
		return nil
	}

	info := &CIInfo{
		Provider:     "github-actions",
		ProviderName: "GitHub Actions",
		CommitSHA:    env("GITHUB_SHA"),
		RepoSlug:     env("GITHUB_REPOSITORY"),
		JobID:        env("GITHUB_JOB"),
		BuildNumber:  env("GITHUB_RUN_NUMBER"),
	}

	serverURL := env("GITHUB_SERVER_URL")
	if serverURL == "" {
		serverURL = "https://github.com"
	}
	if info.RepoSlug != "" {
		info.RepoURL = serverURL + "/" + info.RepoSlug
		runID := env("GITHUB_RUN_ID")
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
