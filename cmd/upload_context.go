package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/drape-io/drape-cli/internal/api"
	"github.com/drape-io/drape-cli/internal/cidetect"
	"github.com/drape-io/drape-cli/internal/exitcode"
	"github.com/drape-io/drape-cli/internal/output"
)

// uploadContext holds the resolved state shared by all upload commands:
// CI info, branch/sha, org/client/repo, and PR number.
type uploadContext struct {
	ci       *cidetect.CIInfo
	branch   string
	sha      string
	prNumber int

	// Available after resolveClient(). Not set during dry-run.
	client   *api.Client
	orgSlug  string
	repoID   int
	repoName string
}

// newUploadContext detects CI, resolves branch/sha, and validates required fields.
// This is phase 1 — enough for dry-run. Call resolveClient() for network operations.
func newUploadContext() (*uploadContext, error) {
	ci := cidetect.Detect(os.Getenv)
	if ci == nil {
		ci = cidetect.DetectFromGit()
	}

	branch := resolveGitContext(flagUploadBranch, ci, func(info *cidetect.CIInfo) string { return info.Branch })
	sha := resolveGitContext(flagUploadSHA, ci, func(info *cidetect.CIInfo) string { return info.CommitSHA })

	if branch == "" {
		return nil, &ExitError{Code: exitcode.UsageError, Err: errMissing("--branch (could not auto-detect)")}
	}
	if sha == "" {
		return nil, &ExitError{Code: exitcode.UsageError, Err: errMissing("--sha (could not auto-detect)")}
	}

	prNumber := 0
	if ci != nil && ci.PRNumber != "" {
		prNumber, _ = strconv.Atoi(ci.PRNumber)
	}

	if ci != nil {
		output.Verbose("Detected CI: %s", ci.ProviderName)
		output.Verbose("  Branch: %s, SHA: %s", branch, sha)
		if ci.IsPullRequest {
			output.Verbose("  PR #%s → %s", ci.PRNumber, ci.TargetBranch)
		}
	}

	return &uploadContext{
		ci:       ci,
		branch:   branch,
		sha:      sha,
		prNumber: prNumber,
	}, nil
}

// resolveClient sets up the API client, org, and repo. Must be called before
// uploadFile or any network operations. Skipped during dry-run.
func (ctx *uploadContext) resolveClient() error {
	// Extract org/repo from CI-detected RepoSlug as fallback values.
	var ciOrg, ciRepo string
	if ctx.ci != nil && ctx.ci.RepoSlug != "" {
		if parts := strings.SplitN(ctx.ci.RepoSlug, "/", 2); len(parts) == 2 {
			ciOrg = parts[0]
			ciRepo = parts[1]
		}
	}

	orgSlug, err := resolveOrg(ciOrg)
	if err != nil {
		return err
	}

	client, err := newClient()
	if err != nil {
		return err
	}

	repoID, repoName, err := resolveRepoID(client, orgSlug, ciRepo)
	if err != nil {
		return err
	}

	ctx.client = client
	ctx.orgSlug = orgSlug
	ctx.repoID = repoID
	ctx.repoName = repoName
	return nil
}

// uploadFile performs the 3-step upload: initiate → presigned PUT → complete.
// Returns the upload ID on success.
func (ctx *uploadContext) uploadFile(uploadType, filename string, data []byte, metadata map[string]any) (int, error) {
	initResp, err := ctx.client.InitiateUpload(ctx.orgSlug, ctx.repoID, api.UploadInitiateRequest{
		UploadType: uploadType,
		Branch:     ctx.branch,
		SHA:        ctx.sha,
		Filename:   filename,
		Metadata:   metadata,
	})
	if err != nil {
		return 0, err
	}
	output.Verbose("Upload ID: %d, uploading to presigned URL...", initResp.UploadID)

	if err := ctx.client.UploadToPresignedURL(initResp.UploadURL, data); err != nil {
		return 0, err
	}
	output.Verbose("File uploaded to object storage")

	if err := ctx.client.CompleteUpload(ctx.orgSlug, ctx.repoID, initResp.UploadID); err != nil {
		return 0, err
	}

	return initResp.UploadID, nil
}

// pollTimeout returns the configured wait timeout as a time.Duration.
func (ctx *uploadContext) pollTimeout() time.Duration {
	return time.Duration(flagUploadTimeout) * time.Second
}

// drapeURL constructs a dashboard URL for the given upload ID.
func (ctx *uploadContext) drapeURL(uploadID int) string {
	baseURL := ctx.client.BaseURL
	return fmt.Sprintf("%s/orgs/%s/repos/%s/uploads/%d", baseURL, ctx.orgSlug, ctx.repoName, uploadID)
}

// resolveGitContext returns the flag value if set, otherwise the CI-detected value.
func resolveGitContext(flagVal string, ci *cidetect.CIInfo, getter func(*cidetect.CIInfo) string) string {
	if flagVal != "" {
		return flagVal
	}
	if ci != nil {
		return getter(ci)
	}
	return ""
}
