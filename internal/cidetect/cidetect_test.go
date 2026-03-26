package cidetect

import "testing"

// envFromMap returns an EnvFunc backed by a map.
func envFromMap(m map[string]string) EnvFunc {
	return func(key string) string {
		return m[key]
	}
}

func TestDetect_GitHubActions(t *testing.T) {
	env := envFromMap(map[string]string{
		"GITHUB_ACTIONS":    "true",
		"GITHUB_SHA":        "abc123",
		"GITHUB_REPOSITORY": "drape-io/webapp",
		"GITHUB_REF":        "refs/heads/main",
		"GITHUB_RUN_ID":     "12345",
		"GITHUB_RUN_NUMBER": "42",
		"GITHUB_JOB":        "test",
		"GITHUB_SERVER_URL": "https://github.com",
	})

	info := Detect(env)
	if info == nil {
		t.Fatal("expected CI info, got nil")
	}
	assertEqual(t, "Provider", "github-actions", info.Provider)
	assertEqual(t, "CommitSHA", "abc123", info.CommitSHA)
	assertEqual(t, "Branch", "main", info.Branch)
	assertEqual(t, "RepoSlug", "drape-io/webapp", info.RepoSlug)
	assertEqual(t, "BuildURL", "https://github.com/drape-io/webapp/actions/runs/12345", info.BuildURL)
	if info.IsPullRequest {
		t.Error("expected IsPullRequest=false")
	}
}

func TestDetect_GitHubActions_PR(t *testing.T) {
	env := envFromMap(map[string]string{
		"GITHUB_ACTIONS":    "true",
		"GITHUB_SHA":        "merge-commit-sha",
		"GITHUB_REPOSITORY": "drape-io/webapp",
		"GITHUB_REF":        "refs/pull/42/merge",
		"GITHUB_HEAD_REF":   "feature-branch",
		"GITHUB_BASE_REF":   "main",
		"GITHUB_SERVER_URL": "https://github.com",
	})

	info := Detect(env)
	if info == nil {
		t.Fatal("expected CI info, got nil")
	}
	if !info.IsPullRequest {
		t.Error("expected IsPullRequest=true")
	}
	assertEqual(t, "PRNumber", "42", info.PRNumber)
	assertEqual(t, "Branch", "feature-branch", info.Branch)
	assertEqual(t, "TargetBranch", "main", info.TargetBranch)
	// For PRs, SHA should still be the merge commit unless overridden
	assertEqual(t, "CommitSHA", "merge-commit-sha", info.CommitSHA)
}

func TestDetect_GitHubActions_PR_HeadSHA(t *testing.T) {
	env := envFromMap(map[string]string{
		"GITHUB_ACTIONS":                       "true",
		"GITHUB_SHA":                           "merge-commit-sha",
		"GITHUB_HEAD_REF":                      "feature-branch",
		"GITHUB_BASE_REF":                      "main",
		"GITHUB_EVENT_PULL_REQUEST_HEAD_SHA":    "actual-head-sha",
		"GITHUB_REPOSITORY":                    "drape-io/webapp",
	})

	info := Detect(env)
	if info == nil {
		t.Fatal("expected CI info, got nil")
	}
	assertEqual(t, "CommitSHA", "actual-head-sha", info.CommitSHA)
}

func TestDetect_GitLabCI(t *testing.T) {
	env := envFromMap(map[string]string{
		"GITLAB_CI":         "true",
		"CI_COMMIT_SHA":     "def456",
		"CI_COMMIT_BRANCH":  "develop",
		"CI_PROJECT_PATH":   "drape/webapp",
		"CI_PROJECT_URL":    "https://gitlab.com/drape/webapp",
		"CI_JOB_URL":        "https://gitlab.com/drape/webapp/-/jobs/789",
		"CI_JOB_ID":         "789",
		"CI_PIPELINE_IID":   "10",
	})

	info := Detect(env)
	if info == nil {
		t.Fatal("expected CI info, got nil")
	}
	assertEqual(t, "Provider", "gitlab-ci", info.Provider)
	assertEqual(t, "CommitSHA", "def456", info.CommitSHA)
	assertEqual(t, "Branch", "develop", info.Branch)
	assertEqual(t, "RepoSlug", "drape/webapp", info.RepoSlug)
}

func TestDetect_GitLab_MR_SHA_Fallback(t *testing.T) {
	env := envFromMap(map[string]string{
		"GITLAB_CI":                            "true",
		"CI_COMMIT_SHA":                        "merge-commit-sha",
		"CI_MERGE_REQUEST_SOURCE_BRANCH_SHA":   "actual-head-sha",
		"CI_MERGE_REQUEST_IID":                 "99",
		"CI_MERGE_REQUEST_TARGET_BRANCH_NAME":  "main",
		"CI_MERGE_REQUEST_SOURCE_BRANCH_NAME":  "feature-x",
	})

	info := Detect(env)
	if info == nil {
		t.Fatal("expected CI info, got nil")
	}
	assertEqual(t, "CommitSHA", "actual-head-sha", info.CommitSHA)
	assertEqual(t, "PRNumber", "99", info.PRNumber)
	if !info.IsPullRequest {
		t.Error("expected IsPullRequest=true")
	}
	assertEqual(t, "TargetBranch", "main", info.TargetBranch)
	assertEqual(t, "Branch", "feature-x", info.Branch)
}

func TestDetect_CircleCI(t *testing.T) {
	env := envFromMap(map[string]string{
		"CIRCLECI":                 "true",
		"CIRCLE_SHA1":              "ccc111",
		"CIRCLE_BRANCH":           "main",
		"CIRCLE_BUILD_URL":        "https://circleci.com/gh/drape/webapp/42",
		"CIRCLE_BUILD_NUM":        "42",
		"CIRCLE_PROJECT_USERNAME": "drape",
		"CIRCLE_PROJECT_REPONAME": "webapp",
	})

	info := Detect(env)
	if info == nil {
		t.Fatal("expected CI info, got nil")
	}
	assertEqual(t, "Provider", "circleci", info.Provider)
	assertEqual(t, "RepoSlug", "drape/webapp", info.RepoSlug)
}

func TestDetect_Buildkite(t *testing.T) {
	env := envFromMap(map[string]string{
		"BUILDKITE":              "true",
		"BUILDKITE_COMMIT":       "bbb222",
		"BUILDKITE_BRANCH":       "main",
		"BUILDKITE_REPO":         "git@github.com:drape/webapp.git",
		"BUILDKITE_BUILD_URL":    "https://buildkite.com/drape/webapp/builds/5",
		"BUILDKITE_BUILD_NUMBER": "5",
	})

	info := Detect(env)
	if info == nil {
		t.Fatal("expected CI info, got nil")
	}
	assertEqual(t, "Provider", "buildkite", info.Provider)
	assertEqual(t, "RepoSlug", "drape/webapp", info.RepoSlug)
}

func TestDetect_Jenkins(t *testing.T) {
	env := envFromMap(map[string]string{
		"JENKINS_URL":  "https://jenkins.example.com/",
		"GIT_COMMIT":   "jjj333",
		"GIT_BRANCH":   "main",
		"BUILD_URL":    "https://jenkins.example.com/job/webapp/1",
		"BUILD_NUMBER": "1",
		"BUILD_ID":     "1",
		"GIT_URL":      "https://github.com/drape/webapp.git",
	})

	info := Detect(env)
	if info == nil {
		t.Fatal("expected CI info, got nil")
	}
	assertEqual(t, "Provider", "jenkins", info.Provider)
	assertEqual(t, "CommitSHA", "jjj333", info.CommitSHA)
	assertEqual(t, "RepoSlug", "drape/webapp", info.RepoSlug)
}

func TestDetect_Azure(t *testing.T) {
	env := envFromMap(map[string]string{
		"TF_BUILD":             "True",
		"BUILD_SOURCEVERSION":  "aaa444",
		"BUILD_SOURCEBRANCH":   "refs/heads/main",
		"BUILD_REPOSITORY_NAME": "drape/webapp",
		"BUILD_BUILDNUMBER":    "20260312.1",
	})

	info := Detect(env)
	if info == nil {
		t.Fatal("expected CI info, got nil")
	}
	assertEqual(t, "Provider", "azure-pipelines", info.Provider)
	assertEqual(t, "Branch", "main", info.Branch)
}

func TestDetect_Travis(t *testing.T) {
	env := envFromMap(map[string]string{
		"TRAVIS":              "true",
		"TRAVIS_COMMIT":       "ttt555",
		"TRAVIS_BRANCH":       "main",
		"TRAVIS_REPO_SLUG":    "drape/webapp",
		"TRAVIS_BUILD_NUMBER": "100",
		"TRAVIS_PULL_REQUEST": "false",
	})

	info := Detect(env)
	if info == nil {
		t.Fatal("expected CI info, got nil")
	}
	assertEqual(t, "Provider", "travis-ci", info.Provider)
	if info.IsPullRequest {
		t.Error("expected IsPullRequest=false for TRAVIS_PULL_REQUEST=false")
	}
}

func TestDetect_Travis_PR(t *testing.T) {
	env := envFromMap(map[string]string{
		"TRAVIS":                      "true",
		"TRAVIS_COMMIT":               "ttt555",
		"TRAVIS_BRANCH":               "main",
		"TRAVIS_PULL_REQUEST":         "7",
		"TRAVIS_PULL_REQUEST_BRANCH":  "feature-x",
		"TRAVIS_REPO_SLUG":            "drape/webapp",
	})

	info := Detect(env)
	if info == nil {
		t.Fatal("expected CI info, got nil")
	}
	if !info.IsPullRequest {
		t.Error("expected IsPullRequest=true")
	}
	assertEqual(t, "PRNumber", "7", info.PRNumber)
	assertEqual(t, "Branch", "feature-x", info.Branch)
	assertEqual(t, "TargetBranch", "main", info.TargetBranch)
}

func TestDetect_Bitbucket(t *testing.T) {
	env := envFromMap(map[string]string{
		"BITBUCKET_BUILD_NUMBER":   "15",
		"BITBUCKET_COMMIT":         "bbb666",
		"BITBUCKET_BRANCH":         "main",
		"BITBUCKET_REPO_FULL_NAME": "drape/webapp",
		"BITBUCKET_WORKSPACE":      "drape",
		"BITBUCKET_REPO_SLUG":      "webapp",
	})

	info := Detect(env)
	if info == nil {
		t.Fatal("expected CI info, got nil")
	}
	assertEqual(t, "Provider", "bitbucket-pipelines", info.Provider)
	assertEqual(t, "BuildURL", "https://bitbucket.org/drape/webapp/addon/pipelines/home#!/results/15", info.BuildURL)
}

func TestDetect_NoCI(t *testing.T) {
	env := envFromMap(map[string]string{})
	info := Detect(env)
	if info != nil {
		t.Errorf("expected nil, got %+v", info)
	}
}

func TestParseGitURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"git@github.com:drape/webapp.git", "drape/webapp"},
		{"https://github.com/drape/webapp.git", "drape/webapp"},
		{"https://github.com/drape/webapp", "drape/webapp"},
		{"", ""},
	}
	for _, tt := range tests {
		got := parseGitURL(tt.input)
		if got != tt.want {
			t.Errorf("parseGitURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func assertEqual(t *testing.T, field, want, got string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}
