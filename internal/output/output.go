// Package output provides formatted CLI output helpers.
package output

import (
	"fmt"
	"io"
	"os"
)

var (
	Stdout io.Writer = os.Stdout
	Stderr io.Writer = os.Stderr
)

var (
	verbose bool
	quiet   bool
)

// SetVerbose enables or disables verbose output.
func SetVerbose(v bool) {
	verbose = v
}

// SetQuiet enables or disables quiet mode. When quiet, Info and Verbose
// produce no output. Error still writes to stderr so diagnostics aren't
// lost. JSON output is unaffected.
func SetQuiet(q bool) {
	quiet = q
}

// Reset restores all output state to defaults. Intended for use in tests.
func Reset() {
	Stdout = os.Stdout
	Stderr = os.Stderr
	verbose = false
	quiet = false
}

// Info prints an informational message to stdout.
func Info(format string, args ...any) {
	if quiet {
		return
	}
	_, _ = fmt.Fprintf(Stdout, format+"\n", args...)
}

// Error prints an error message to stderr.
// Errors are always printed, even in quiet mode, so diagnostics aren't lost.
func Error(format string, args ...any) {
	_, _ = fmt.Fprintf(Stderr, "Error: "+format+"\n", args...)
}

// Verbose prints a message only when verbose mode is enabled.
func Verbose(format string, args ...any) {
	if quiet {
		return
	}
	if verbose {
		_, _ = fmt.Fprintf(Stderr, "[verbose] "+format+"\n", args...)
	}
}
