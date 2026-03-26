package cidetect

func detectGitLab(env EnvFunc) *CIInfo {
	if env("GITLAB_CI") != "true" {
		return nil
	}

	info := &CIInfo{
		Provider:     "gitlab-ci",
		ProviderName: "GitLab CI",
		BuildURL:     env("CI_JOB_URL"),
		BuildNumber:  env("CI_PIPELINE_IID"),
		Branch:       env("CI_COMMIT_BRANCH"),
		Tag:          env("CI_COMMIT_TAG"),
		RepoURL:      env("CI_PROJECT_URL"),
		RepoSlug:     env("CI_PROJECT_PATH"),
		JobID:        env("CI_JOB_ID"),
	}

	// 3-level SHA fallback chain:
	// 1. CI_COMMIT_SHA (always set, but might be merge commit for MRs)
	// 2. CI_MERGE_REQUEST_SOURCE_BRANCH_SHA (actual head for MR pipelines)
	// 3. CI_BUILD_REF (legacy, same as CI_COMMIT_SHA)
	info.CommitSHA = env("CI_COMMIT_SHA")
	if mrSHA := env("CI_MERGE_REQUEST_SOURCE_BRANCH_SHA"); mrSHA != "" {
		info.CommitSHA = mrSHA
	}

	// Merge request detection
	if mrIID := env("CI_MERGE_REQUEST_IID"); mrIID != "" {
		info.PRNumber = mrIID
		info.IsPullRequest = true
		info.TargetBranch = env("CI_MERGE_REQUEST_TARGET_BRANCH_NAME")
		if info.Branch == "" {
			info.Branch = env("CI_MERGE_REQUEST_SOURCE_BRANCH_NAME")
		}
	}

	return info
}
