package cmd

// UploadResult is the unified JSON output for all upload commands.
// Single-file commands (coverage, lint) have one entry in Uploads.
// Multi-file commands (tests, scan) have one entry per file.
type UploadResult struct {
	Uploads       []UploadEntry `json:"uploads"`
	FilesMatched  int           `json:"files_matched"`
	FilesUploaded int           `json:"files_uploaded"`
}

// UploadEntry is a single file entry within UploadResult.
// Result holds the type-specific status response (e.g. *api.CoverageStatusResponse).
type UploadEntry struct {
	Filename string `json:"filename"`
	UploadID int    `json:"upload_id"`
	DrapeURL string `json:"drape_url,omitempty"`
	Result   any    `json:"result,omitempty"`
}

// DryRunResult is the JSON output for commands run with --dry-run.
type DryRunResult struct {
	DryRun bool     `json:"dry_run"`
	Files  []string `json:"files"`
}

// TestsDryRunResult is the JSON output for "upload tests --dry-run",
// which includes parsed test counts per file.
type TestsDryRunResult struct {
	DryRun bool              `json:"dry_run"`
	Files  []TestsDryRunFile `json:"files"`
}

// TestsDryRunFile is a single file entry within TestsDryRunResult.
type TestsDryRunFile struct {
	Filename string `json:"filename"`
	Total    int    `json:"total"`
	Passed   int    `json:"passed"`
	Failed   int    `json:"failed"`
	Skipped  int    `json:"skipped"`
	Errored  int    `json:"errored"`
}
