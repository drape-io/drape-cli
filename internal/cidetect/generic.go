package cidetect

import (
	"os/exec"
	"strings"
)

// DetectFromGit attempts to get branch and SHA from git commands.
// Used as a fallback when no CI environment is detected.
func DetectFromGit() *CIInfo {
	sha := gitCommand("rev-parse", "HEAD")
	branch := gitCommand("rev-parse", "--abbrev-ref", "HEAD")

	if sha == "" && branch == "" {
		return nil
	}

	// Parse repo slug from the git remote origin URL.
	remoteURL := gitCommand("remote", "get-url", "origin")
	repoSlug := parseGitURL(remoteURL)

	return &CIInfo{
		Provider:     "local",
		ProviderName: "Local",
		CommitSHA:    sha,
		Branch:       branch,
		RepoSlug:     repoSlug,
	}
}

func gitCommand(args ...string) string {
	// Only known safe subcommands are passed here (rev-parse, remote).
	// The binary is hardcoded to "git" — args are not user-controlled.
	cmd := exec.Command("git", args...) //nolint:gosec // G204: args are hardcoded callers, not user input
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
