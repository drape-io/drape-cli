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

	return &CIInfo{
		Provider:     "local",
		ProviderName: "Local",
		CommitSHA:    sha,
		Branch:       branch,
	}
}

func gitCommand(args ...string) string {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
