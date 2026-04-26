package cmd

import (
	"time"

	"github.com/spf13/cobra"
)

// Shared flags for all upload subcommands.
var (
	flagUploadBranch      string
	flagUploadSHA         string
	flagUploadWait        bool
	flagUploadWaitTimeout time.Duration
)

var uploadCmd = &cobra.Command{
	Use:   "upload",
	Short: "Upload results to Drape (tests, coverage, lint, scan)",
}

func init() {
	uploadCmd.PersistentFlags().StringVar(&flagUploadBranch, "branch", "", "Git branch (auto-detected from CI)")
	uploadCmd.PersistentFlags().StringVar(&flagUploadSHA, "sha", "", "Git commit SHA (auto-detected from CI)")
	uploadCmd.PersistentFlags().BoolVar(&flagUploadWait, "wait", true, "Wait for server-side processing")
	uploadCmd.PersistentFlags().DurationVar(&flagUploadWaitTimeout, "wait-timeout", 3*time.Minute, "Max wait time as a Go duration (e.g. 90s, 3m, 10m)")

	rootCmd.AddCommand(uploadCmd)
}
