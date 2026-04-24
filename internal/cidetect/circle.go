package cidetect

func detectCircleCI(env EnvFunc) *CIInfo {
	if env("CIRCLECI") != "true" {
		return nil
	}

	info := &CIInfo{
		Provider:      "circleci",
		ProviderName:  "CircleCI",
		CommitSHA:     env("CIRCLE_SHA1"),
		Branch:        env("CIRCLE_BRANCH"),
		Tag:           env("CIRCLE_TAG"),
		BuildURL:      env("CIRCLE_BUILD_URL"),
		BuildNumber:   env("CIRCLE_BUILD_NUM"),
		RepoURL:       env("CIRCLE_REPOSITORY_URL"),
		JobID:         env("CIRCLE_WORKFLOW_JOB_ID"),
		ProviderRunID: env("CIRCLE_WORKFLOW_ID"),
	}

	// Repo slug from project components
	username := env("CIRCLE_PROJECT_USERNAME")
	reponame := env("CIRCLE_PROJECT_REPONAME")
	if username != "" && reponame != "" {
		info.RepoSlug = username + "/" + reponame
	}

	// PR detection
	if prNum := env("CIRCLE_PR_NUMBER"); prNum != "" {
		info.PRNumber = prNum
		info.IsPullRequest = true
	} else if env("CIRCLE_PULL_REQUEST") != "" {
		info.IsPullRequest = true
	}

	return info
}
