package cmd

import (
	"fmt"
	"os"
)

// resolveFlag returns the flag value if non-empty, otherwise the env var value.
func resolveFlag(flagVal, envKey string) string {
	if flagVal != "" {
		return flagVal
	}
	return os.Getenv(envKey)
}

func errMissing(name string) error {
	return fmt.Errorf("required: %s", name)
}

// applyDrapeRunIDMetadata resolves --drape-run-id (or DRAPE_RUN_ID) and writes
// it to the metadata map. When set and no explicit --group was provided, it also
// sets the group to "drape:{run_id}" so the server can isolate triggered-run data.
// The wire contract is metadata["run_id"] + DRAPE_RUN_ID env var, independent of
// the CLI flag name.
func applyDrapeRunIDMetadata(metadata map[string]any, runIDFlag string, groups []string) {
	runID := resolveFlag(runIDFlag, "DRAPE_RUN_ID")
	if runID == "" {
		return
	}
	metadata["run_id"] = runID
	if len(groups) == 0 {
		metadata["group"] = "drape:" + runID
	}
}
