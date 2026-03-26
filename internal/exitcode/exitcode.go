// Package exitcode defines CLI exit codes.
package exitcode

const (
	Success            = 0
	TestFailure        = 1
	UsageError         = 2
	UploadError        = 3
	Timeout            = 4
	ParseError         = 5
	CoverageRegression = 6
	LintFailure        = 7
	ScanFailure        = 8
)
