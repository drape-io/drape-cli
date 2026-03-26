package cmd

import "github.com/spf13/cobra"

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate test result files locally",
}

func init() {
	rootCmd.AddCommand(validateCmd)
}
