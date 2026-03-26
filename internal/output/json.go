package output

import (
	"encoding/json"
	"fmt"
	"os"
)

// JSON serializes v as indented JSON and writes it to the real stdout
// (os.Stdout, not the Stdout variable which may have been redirected to stderr).
// This should be called at most once per command invocation.
func JSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON output: %w", err)
	}
	_, err = fmt.Fprintln(os.Stdout, string(data))
	return err
}
