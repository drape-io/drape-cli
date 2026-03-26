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

var verbose bool

// SetVerbose enables or disables verbose output.
func SetVerbose(v bool) {
	verbose = v
}

// Info prints an informational message to stdout.
func Info(format string, args ...any) {
	_, _ = fmt.Fprintf(Stdout, format+"\n", args...)
}

// Error prints an error message to stderr.
func Error(format string, args ...any) {
	_, _ = fmt.Fprintf(Stderr, "Error: "+format+"\n", args...)
}

// Verbose prints a message only when verbose mode is enabled.
func Verbose(format string, args ...any) {
	if verbose {
		_, _ = fmt.Fprintf(Stderr, "[verbose] "+format+"\n", args...)
	}
}
