package main

import (
	"os"

	"github.com/drape-io/drape-cli/cmd"
)

// Set via ldflags at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetVersionInfo(version, commit, date)
	code := cmd.Execute()
	os.Exit(code)
}
