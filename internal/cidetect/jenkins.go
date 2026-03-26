package cidetect

func detectJenkins(env EnvFunc) *CIInfo {
	jenkinsURL := env("JENKINS_URL")
	if jenkinsURL == "" {
		jenkinsURL = env("BUILD_URL")
		if jenkinsURL == "" || env("JENKINS_HOME") == "" {
			return nil
		}
	}

	info := &CIInfo{
		Provider:     "jenkins",
		ProviderName: "Jenkins",
		CommitSHA:    env("GIT_COMMIT"),
		Branch:       env("GIT_BRANCH"),
		BuildURL:     env("BUILD_URL"),
		BuildNumber:  env("BUILD_NUMBER"),
		JobID:        env("BUILD_ID"),
	}

	// Parse repo slug from GIT_URL
	info.RepoSlug = parseGitURL(env("GIT_URL"))

	// PR detection (varies by Jenkins plugin)
	if prNum := env("CHANGE_ID"); prNum != "" {
		info.PRNumber = prNum
		info.IsPullRequest = true
		info.TargetBranch = env("CHANGE_TARGET")
	}

	return info
}
