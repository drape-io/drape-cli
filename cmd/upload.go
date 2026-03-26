package cmd

import "github.com/spf13/cobra"

// Shared flags for all upload subcommands.
var (
	flagUploadBranch  string
	flagUploadSHA     string
	flagUploadWait    bool
	flagUploadTimeout int
)

var uploadCmd = &cobra.Command{
	Use:   "upload",
	Short: "Upload results to Drape (tests, coverage, lint, scan)",
}

func init() {
	uploadCmd.PersistentFlags().StringVar(&flagUploadBranch, "branch", "", "Git branch (auto-detected from CI)")
	uploadCmd.PersistentFlags().StringVar(&flagUploadSHA, "sha", "", "Git commit SHA (auto-detected from CI)")
	uploadCmd.PersistentFlags().BoolVar(&flagUploadWait, "wait", true, "Wait for server-side processing")
	uploadCmd.PersistentFlags().IntVar(&flagUploadTimeout, "timeout", 120, "Max wait time in seconds")

	rootCmd.AddCommand(uploadCmd)
}
