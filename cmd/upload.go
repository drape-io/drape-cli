package cmd

import "github.com/spf13/cobra"

var uploadCmd = &cobra.Command{
	Use:   "upload",
	Short: "Upload results to Drape (tests, coverage, lint, scan)",
}

func init() {
	rootCmd.AddCommand(uploadCmd)
}
