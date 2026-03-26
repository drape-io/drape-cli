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
