package cmd

import (
	"github.com/spf13/cobra"

	"github.com/drape-io/drape-cli/internal/output"
)

var (
	cliVersion = "dev"
	cliCommit  = "none"
	cliDate    = "unknown"
)

// SetVersionInfo sets the version information (called from main).
func SetVersionInfo(version, commit, date string) {
	cliVersion = version
	cliCommit = commit
	cliDate = date
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the CLI version",
	Run: func(cmd *cobra.Command, args []string) {
		output.Info("drape version %s (commit: %s, built: %s)", cliVersion, cliCommit, cliDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
