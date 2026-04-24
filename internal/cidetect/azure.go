package cidetect

import "strings"

func detectAzure(env EnvFunc) *CIInfo {
	if env("TF_BUILD") != "True" {
		return nil
	}

	info := &CIInfo{
		Provider:      "azure-pipelines",
		ProviderName:  "Azure Pipelines",
		CommitSHA:     env("BUILD_SOURCEVERSION"),
		BuildURL:      env("SYSTEM_TEAMFOUNDATIONSERVERURI") + env("SYSTEM_TEAMPROJECT") + "/_build/results?buildId=" + env("BUILD_BUILDID"),
		BuildNumber:   env("BUILD_BUILDNUMBER"),
		RepoURL:       env("BUILD_REPOSITORY_URI"),
		RepoSlug:      env("BUILD_REPOSITORY_NAME"),
		JobID:         env("SYSTEM_JOBID"),
		ProviderRunID: env("BUILD_BUILDID"),
	}

	// Strip refs/heads/ from branch
	branch := env("BUILD_SOURCEBRANCH")
	branch = strings.TrimPrefix(branch, "refs/heads/")
	branch = strings.TrimPrefix(branch, "refs/tags/")
	info.Branch = branch

	// PR detection
	if prNum := env("SYSTEM_PULLREQUEST_PULLREQUESTNUMBER"); prNum != "" {
		info.PRNumber = prNum
		info.IsPullRequest = true
		target := env("SYSTEM_PULLREQUEST_TARGETBRANCH")
		info.TargetBranch = strings.TrimPrefix(target, "refs/heads/")
	}

	return info
}
