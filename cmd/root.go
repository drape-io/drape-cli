// Package cmd implements the CLI commands.
package cmd

import (
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drape-io/drape-cli/internal/api"
	"github.com/drape-io/drape-cli/internal/exitcode"
	"github.com/drape-io/drape-cli/internal/output"
)

// Global flags
var (
	flagOrg    string
	flagRepo   string
	flagToken  string
	flagAPIURL string
	flagVerbose bool
	flagDryRun  bool
)

var rootCmd = &cobra.Command{
	Use:   "drape",
	Short: "Drape CLI — upload test results and coverage to Drape",
	Long:  "The Drape CLI integrates your CI pipeline with Drape for test analytics, flakiness detection, and quarantine management.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		output.SetVerbose(flagVerbose)
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagOrg, "org", "", "Organization slug (env: DRAPE_ORG)")
	rootCmd.PersistentFlags().StringVar(&flagRepo, "repo", "", "Repository name (env: DRAPE_REPO)")
	rootCmd.PersistentFlags().StringVar(&flagToken, "token", "", "API token (env: DRAPE_TOKEN)")
	rootCmd.PersistentFlags().StringVar(&flagAPIURL, "api-url", "", "API base URL (env: DRAPE_API_URL, default: https://app.drape.io)")
	rootCmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "Verbose logging")
	rootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "Parse and validate locally, don't upload")
}

// Execute runs the root command and returns an exit code.
func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		// Check if the error wraps an exit code
		if coded, ok := err.(*ExitError); ok {
			output.Error("%v", coded.Err)
			return coded.Code
		}
		output.Error("%v", err)
		if hint := enhanceCobraError(err); hint != "" {
			output.Error("%s", hint)
		}
		return exitcode.UsageError
	}
	return exitcode.Success
}

// argsErrorRe matches Cobra's "accepts N arg(s), received M" errors.
var argsErrorRe = regexp.MustCompile(`accepts (?:at most )?\d+ arg\(s\), received (\d+)`)

// enhanceCobraError detects common Cobra error patterns and returns a helpful hint.
func enhanceCobraError(err error) string {
	msg := err.Error()

	if argsErrorRe.MatchString(msg) {
		return "hint: this often happens when a flag is passed twice (e.g. --branch used twice),\n" +
			"      causing the second occurrence to consume the next flag's value as its argument.\n" +
			"      Check your command for duplicate flags."
	}

	if strings.Contains(msg, "unknown flag") || strings.Contains(msg, "unknown shorthand flag") {
		// Extract the flag name for a better message
		return "hint: run the command with --help to see available flags."
	}

	return ""
}

// ExitError wraps an error with a specific exit code.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	return e.Err.Error()
}

// newClient creates an API client from global flags, resolving env var defaults.
func newClient() (*api.Client, error) {
	token := resolveFlag(flagToken, "DRAPE_TOKEN")
	if token == "" {
		return nil, &ExitError{Code: exitcode.UsageError, Err: errMissing("--token or DRAPE_TOKEN")}
	}

	apiURL := resolveFlag(flagAPIURL, "DRAPE_API_URL")
	if apiURL == "" {
		apiURL = "https://app.drape.io"
	}

	client, err := api.NewClient(apiURL, token)
	if err != nil {
		return nil, &ExitError{Code: exitcode.UsageError, Err: err}
	}
	return client, nil
}

// resolveRepoID resolves the --repo flag to a numeric repo ID via the API.
func resolveRepoID(client *api.Client, orgSlug string) (int, error) {
	repoName := resolveFlag(flagRepo, "DRAPE_REPO")
	if repoName == "" {
		return 0, &ExitError{Code: exitcode.UsageError, Err: errMissing("--repo or DRAPE_REPO")}
	}

	output.Verbose("Looking up repository %q in org %q...", repoName, orgSlug)
	repo, err := client.LookupRepo(orgSlug, repoName)
	if err != nil {
		return 0, &ExitError{Code: exitcode.UploadError, Err: err}
	}
	output.Verbose("Resolved repo %q to ID %d", repoName, repo.ID)
	return repo.ID, nil
}

func resolveOrg() (string, error) {
	org := resolveFlag(flagOrg, "DRAPE_ORG")
	if org == "" {
		return "", &ExitError{Code: exitcode.UsageError, Err: errMissing("--org or DRAPE_ORG")}
	}
	return org, nil
}
