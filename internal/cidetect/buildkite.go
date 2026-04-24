package cidetect

import "strings"

func detectBuildkite(env EnvFunc) *CIInfo {
	if env("BUILDKITE") != "true" {
		return nil
	}

	info := &CIInfo{
		Provider:      "buildkite",
		ProviderName:  "Buildkite",
		CommitSHA:     env("BUILDKITE_COMMIT"),
		Branch:        env("BUILDKITE_BRANCH"),
		Tag:           env("BUILDKITE_TAG"),
		BuildURL:      env("BUILDKITE_BUILD_URL"),
		BuildNumber:   env("BUILDKITE_BUILD_NUMBER"),
		JobID:         env("BUILDKITE_JOB_ID"),
		ProviderRunID: env("BUILDKITE_BUILD_ID"),
	}

	// Parse repo slug from BUILDKITE_REPO (git URL)
	info.RepoSlug = parseGitURL(env("BUILDKITE_REPO"))

	// PR detection
	if prNum := env("BUILDKITE_PULL_REQUEST"); prNum != "" && prNum != "false" {
		info.PRNumber = prNum
		info.IsPullRequest = true
		info.TargetBranch = env("BUILDKITE_PULL_REQUEST_BASE_BRANCH")
	}

	return info
}

// parseGitURL extracts owner/repo from various git URL formats.
func parseGitURL(url string) string {
	if url == "" {
		return ""
	}
	// Handle SSH URLs: git@github.com:owner/repo.git
	if strings.Contains(url, ":") && strings.Contains(url, "@") {
		parts := strings.SplitN(url, ":", 2)
		if len(parts) == 2 {
			slug := parts[1]
			slug = strings.TrimSuffix(slug, ".git")
			return slug
		}
	}
	// Handle HTTPS URLs: https://github.com/owner/repo.git
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(url, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	return ""
}
