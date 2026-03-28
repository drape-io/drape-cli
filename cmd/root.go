// Package cmd implements the CLI commands.
package cmd

import (
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drape-io/drape-cli/internal/api"
	"github.com/drape-io/drape-cli/internal/exitcode"
	"github.com/drape-io/drape-cli/internal/output"
)

// Global flags
var (
	flagOrg     string
	flagRepo    string
	flagAPIKey  string
	flagAPIURL  string
	flagVerbose bool
	flagDryRun  bool
	flagJSON    bool
	flagQuiet   bool
)

// pendingJSON holds the result to be emitted as JSON after command execution.
// Commands call setResult() to populate this; Execute() emits it.
var pendingJSON any

// setResult stores a result for JSON emission after the command completes.
// This is the single place commands register their output; Execute() handles
// the actual emission, ensuring JSON is written even when commands return errors.
func setResult(v any) {
	pendingJSON = v
}

var rootCmd = &cobra.Command{
	Use:   "drape",
	Short: "Drape CLI — upload test results and coverage to Drape",
	Long:  "The Drape CLI integrates your CI pipeline with Drape for test analytics, flakiness detection, and suppression management.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if flagQuiet {
			flagJSON = true
		}
		output.SetVerbose(flagVerbose)
		output.SetQuiet(flagQuiet)
		if flagJSON {
			output.Stdout = os.Stderr
		}
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagOrg, "org", "", "Organization slug (env: DRAPE_ORG)")
	rootCmd.PersistentFlags().StringVar(&flagRepo, "repo", "", "Repository name (env: DRAPE_REPO)")
	rootCmd.PersistentFlags().StringVar(&flagAPIKey, "api-key", "", "API key (env: DRAPE_API_KEY)")
	rootCmd.PersistentFlags().StringVar(&flagAPIURL, "api-url", "", "API base URL (env: DRAPE_API_URL, default: https://app.drape.io)")
	rootCmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "Verbose logging")
	rootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "Parse and validate locally, don't upload")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output results as JSON to stdout")
	rootCmd.PersistentFlags().BoolVar(&flagQuiet, "quiet", false, "Suppress all human-readable output (implies --json)")
}

// Execute runs the root command and returns an exit code.
func Execute() int {
	pendingJSON = nil

	err := rootCmd.Execute()

	// Emit JSON result if available — even on error, so the consumer
	// can parse partial results (e.g. scan with policy failures).
	if flagJSON && pendingJSON != nil {
		if jsonErr := output.JSON(pendingJSON); jsonErr != nil {
			output.Error("failed to write JSON output: %v", jsonErr)
		}
	}

	if err != nil {
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
	apiKey := resolveFlag(flagAPIKey, "DRAPE_API_KEY")
	if apiKey == "" {
		return nil, &ExitError{Code: exitcode.UsageError, Err: errMissing("--api-key or DRAPE_API_KEY")}
	}

	apiURL := resolveFlag(flagAPIURL, "DRAPE_API_URL")
	if apiURL == "" {
		apiURL = "https://app.drape.io"
	}

	client, err := api.NewClient(apiURL, apiKey)
	if err != nil {
		return nil, &ExitError{Code: exitcode.UsageError, Err: err}
	}
	return client, nil
}

// resolveRepoID resolves the --repo flag to a numeric repo ID via the API.
// ciFallback is the repo name extracted from CI-detected RepoSlug (may be empty).
func resolveRepoID(client *api.Client, orgSlug, ciFallback string) (int, string, error) {
	repoName := resolveFlag(flagRepo, "DRAPE_REPO")
	if repoName == "" && ciFallback != "" {
		output.Verbose("Using CI-detected repo: %s", ciFallback)
		repoName = ciFallback
	}
	if repoName == "" {
		return 0, "", &ExitError{Code: exitcode.UsageError, Err: errMissing("--repo, DRAPE_REPO, or CI-detected repo slug")}
	}

	output.Verbose("Looking up repository %q in org %q...", repoName, orgSlug)
	repo, err := client.LookupRepo(orgSlug, repoName)
	if err != nil {
		return 0, "", &ExitError{Code: exitcode.UploadError, Err: err}
	}
	output.Verbose("Resolved repo %q to ID %d", repoName, repo.ID)
	return repo.ID, repoName, nil
}

// resolveOrg resolves the org slug from flags, env vars, or CI-detected fallback.
func resolveOrg(ciFallback string) (string, error) {
	org := resolveFlag(flagOrg, "DRAPE_ORG")
	if org == "" && ciFallback != "" {
		output.Verbose("Using CI-detected org: %s", ciFallback)
		org = ciFallback
	}
	if org == "" {
		return "", &ExitError{Code: exitcode.UsageError, Err: errMissing("--org, DRAPE_ORG, or CI-detected repo slug")}
	}
	return org, nil
}
