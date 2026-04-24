package cidetect

func detectBitbucket(env EnvFunc) *CIInfo {
	if env("BITBUCKET_BUILD_NUMBER") == "" {
		return nil
	}

	info := &CIInfo{
		Provider:      "bitbucket-pipelines",
		ProviderName:  "Bitbucket Pipelines",
		CommitSHA:     env("BITBUCKET_COMMIT"),
		Branch:        env("BITBUCKET_BRANCH"),
		Tag:           env("BITBUCKET_TAG"),
		BuildNumber:   env("BITBUCKET_BUILD_NUMBER"),
		RepoSlug:      env("BITBUCKET_REPO_FULL_NAME"),
		JobID:         env("BITBUCKET_STEP_UUID"),
		ProviderRunID: env("BITBUCKET_BUILD_NUMBER"),
	}

	// Compose build URL
	workspace := env("BITBUCKET_WORKSPACE")
	repoSlug := env("BITBUCKET_REPO_SLUG")
	if workspace != "" && repoSlug != "" {
		info.RepoURL = "https://bitbucket.org/" + workspace + "/" + repoSlug
		if info.BuildNumber != "" {
			info.BuildURL = info.RepoURL + "/addon/pipelines/home#!/results/" + info.BuildNumber
		}
	}

	// PR detection
	if prID := env("BITBUCKET_PR_ID"); prID != "" {
		info.PRNumber = prID
		info.IsPullRequest = true
		info.TargetBranch = env("BITBUCKET_PR_DESTINATION_BRANCH")
	}

	return info
}
