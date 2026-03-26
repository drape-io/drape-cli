package cidetect

func detectTravis(env EnvFunc) *CIInfo {
	if env("TRAVIS") != "true" {
		return nil
	}

	info := &CIInfo{
		Provider:     "travis-ci",
		ProviderName: "Travis CI",
		CommitSHA:    env("TRAVIS_COMMIT"),
		Branch:       env("TRAVIS_BRANCH"),
		Tag:          env("TRAVIS_TAG"),
		BuildURL:     env("TRAVIS_BUILD_WEB_URL"),
		BuildNumber:  env("TRAVIS_BUILD_NUMBER"),
		RepoSlug:     env("TRAVIS_REPO_SLUG"),
		JobID:        env("TRAVIS_JOB_ID"),
	}

	// PR detection: TRAVIS_PULL_REQUEST is a number or "false"
	if prNum := env("TRAVIS_PULL_REQUEST"); prNum != "" && prNum != "false" {
		info.PRNumber = prNum
		info.IsPullRequest = true
		// When it's a PR, TRAVIS_BRANCH is the target branch
		info.TargetBranch = env("TRAVIS_BRANCH")
		// The source branch is in TRAVIS_PULL_REQUEST_BRANCH
		if srcBranch := env("TRAVIS_PULL_REQUEST_BRANCH"); srcBranch != "" {
			info.Branch = srcBranch
		}
	}

	return info
}
