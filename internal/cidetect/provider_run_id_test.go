package cidetect

import "testing"

// TestProviderRunID_AllDetectors verifies that each CI provider's detector
// populates CIInfo.ProviderRunID from the appropriate env var, so --shard-key
// auto-derives without the user passing anything. Jenkins is intentionally
// omitted — it has no reliably shared run ID across matrix children.
func TestProviderRunID_AllDetectors(t *testing.T) {
	cases := []struct {
		name     string
		env      map[string]string
		wantID   string
		wantProv string
	}{
		{
			name:     "github-actions",
			env:      map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_RUN_ID": "42"},
			wantID:   "42",
			wantProv: "github-actions",
		},
		{
			name:     "gitlab-ci",
			env:      map[string]string{"GITLAB_CI": "true", "CI_PIPELINE_ID": "7788"},
			wantID:   "7788",
			wantProv: "gitlab-ci",
		},
		{
			name:     "circleci",
			env:      map[string]string{"CIRCLECI": "true", "CIRCLE_WORKFLOW_ID": "wf-abc-123"},
			wantID:   "wf-abc-123",
			wantProv: "circleci",
		},
		{
			name:     "buildkite",
			env:      map[string]string{"BUILDKITE": "true", "BUILDKITE_BUILD_ID": "build-uuid"},
			wantID:   "build-uuid",
			wantProv: "buildkite",
		},
		{
			name:     "azure-pipelines",
			env:      map[string]string{"TF_BUILD": "True", "BUILD_BUILDID": "999"},
			wantID:   "999",
			wantProv: "azure-pipelines",
		},
		{
			name:     "travis-ci",
			env:      map[string]string{"TRAVIS": "true", "TRAVIS_BUILD_ID": "555"},
			wantID:   "555",
			wantProv: "travis-ci",
		},
		{
			name:     "bitbucket-pipelines",
			env:      map[string]string{"BITBUCKET_BUILD_NUMBER": "42"},
			wantID:   "42",
			wantProv: "bitbucket-pipelines",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := Detect(envFromMap(tc.env))
			if info == nil {
				t.Fatalf("expected CI info, got nil")
			}
			if info.Provider != tc.wantProv {
				t.Errorf("Provider = %q; want %q", info.Provider, tc.wantProv)
			}
			if info.ProviderRunID != tc.wantID {
				t.Errorf("ProviderRunID = %q; want %q", info.ProviderRunID, tc.wantID)
			}
		})
	}
}
