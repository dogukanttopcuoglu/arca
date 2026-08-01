package diagnostics

import (
	"sort"
	"time"

	"arca/internal/pdfinspector/model"
)

// DiagnosticOptions encapsulates input parameters for building pipeline diagnostics metrics.
type DiagnosticOptions struct {
	Status           string
	Warnings         []string
	Errors           []string
	SkippedPages     []int
	RetryCount       int
	StartTime        int64
	ExtractionEngine string
	ExtractionVer    string
}

// Aggregator defines the interface for building and collecting inspection diagnostics.
type Aggregator interface {
	BuildDiagnostics(status string, warnings, errors []string, startTime int64) model.Diagnostics
	BuildDiagnosticsWithOptions(opts DiagnosticOptions) model.Diagnostics
}

// DefaultAggregator implements Aggregator.
type DefaultAggregator struct{}

// NewAggregator creates a new Aggregator instance.
func NewAggregator() *DefaultAggregator {
	return &DefaultAggregator{}
}

// BuildDiagnostics compiles execution metrics into a Diagnostics struct.
func (a *DefaultAggregator) BuildDiagnostics(status string, warnings, errors []string, startTime int64) model.Diagnostics {
	return a.BuildDiagnosticsWithOptions(DiagnosticOptions{
		Status:    status,
		Warnings:  warnings,
		Errors:    errors,
		StartTime: startTime,
	})
}

// BuildDiagnosticsWithOptions compiles execution metrics from DiagnosticOptions into a Diagnostics struct.
func (a *DefaultAggregator) BuildDiagnosticsWithOptions(opts DiagnosticOptions) model.Diagnostics {
	engine := opts.ExtractionEngine
	if engine == "" {
		engine = "firecrawl"
	}

	ver := opts.ExtractionVer
	if ver == "" {
		ver = "1.0.0"
	}

	var duration int64
	if opts.StartTime > 0 {
		duration = time.Now().UnixMilli() - opts.StartTime
		if duration < 0 {
			duration = 0
		}
	}

	warnings := deduplicateStrings(opts.Warnings)
	errorsList := deduplicateStrings(opts.Errors)
	skippedPages := deduplicateInts(opts.SkippedPages)

	retryCount := opts.RetryCount
	if retryCount < 0 {
		retryCount = 0
	}

	status := opts.Status
	if status == "" {
		status = model.StatusSuccess
	}

	// Graceful degradation auto-degradation logic:
	// If pipeline execution completed without fatal failure, but warnings or skipped pages exist,
	// degrade status from "success" to "partial_success".
	if status == model.StatusSuccess {
		if len(warnings) > 0 || len(skippedPages) > 0 {
			status = model.StatusPartialSuccess
		}
	}

	return model.Diagnostics{
		Status:           status,
		ExtractionEngine: engine,
		ExtractionVer:    ver,
		ProcessingTimeMs: duration,
		Warnings:         warnings,
		Errors:           errorsList,
		SkippedPages:     skippedPages,
		RetryCount:       retryCount,
	}
}

func deduplicateStrings(input []string) []string {
	if len(input) == 0 {
		return []string{}
	}
	seen := make(map[string]bool)
	result := make([]string, 0, len(input))
	for _, item := range input {
		if item != "" && !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

func deduplicateInts(input []int) []int {
	if len(input) == 0 {
		return []int{}
	}
	seen := make(map[int]bool)
	result := make([]int, 0, len(input))
	for _, item := range input {
		if item > 0 && !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	sort.Ints(result)
	return result
}
