package model

// ExtractionErrorSeverity defines error log levels for asset extraction processing.
type ExtractionErrorSeverity string

const (
	SeverityWarning  ExtractionErrorSeverity = "warning"
	SeverityCritical ExtractionErrorSeverity = "critical"
)

// SourceLocation defines line and character position bounds in source text.
type SourceLocation struct {
	StartOffset int `json:"startOffset"`
	EndOffset   int `json:"endOffset"`
	StartLine   int `json:"startLine"`
	EndLine     int `json:"endLine"`
}

// Diagnostics records execution metrics, warnings, errors, and degradation details.
type Diagnostics struct {
	Status           string   `json:"status"`
	ExtractionEngine string   `json:"extractionEngine"`
	ExtractionVer    string   `json:"extractionVersion"`
	ProcessingTimeMs int64    `json:"processingTimeMs"`
	Warnings         []string `json:"warnings"`
	Errors           []string `json:"errors"`
	SkippedPages     []int    `json:"skippedPages"`
	RetryCount       int      `json:"retryCount"`
}
