package diagnostics_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"arca/internal/pdfinspector/diagnostics"
	"arca/internal/pdfinspector/model"
)

func TestAggregator_BuildDiagnosticsWithOptions(t *testing.T) {
	agg := diagnostics.NewAggregator()
	startTime := time.Now().Add(-50 * time.Millisecond).UnixMilli()

	t.Run("success without warnings or skipped pages", func(t *testing.T) {
		diag := agg.BuildDiagnosticsWithOptions(diagnostics.DiagnosticOptions{
			Status:    model.StatusSuccess,
			StartTime: startTime,
		})

		if diag.Status != model.StatusSuccess {
			t.Errorf("expected status %q, got %q", model.StatusSuccess, diag.Status)
		}
		if diag.ProcessingTimeMs < 0 {
			t.Errorf("expected processingTimeMs >= 0, got %d", diag.ProcessingTimeMs)
		}
		if diag.ExtractionEngine != "firecrawl" {
			t.Errorf("expected engine 'firecrawl', got %q", diag.ExtractionEngine)
		}
		if diag.ExtractionVer != "1.0.0" {
			t.Errorf("expected version '1.0.0', got %q", diag.ExtractionVer)
		}
		if len(diag.Warnings) != 0 || len(diag.Errors) != 0 || len(diag.SkippedPages) != 0 {
			t.Errorf("expected empty slices, got warnings=%v, errors=%v, skipped=%v", diag.Warnings, diag.Errors, diag.SkippedPages)
		}
		if err := diag.Validate(); err != nil {
			t.Fatalf("diagnostics validation failed: %v", err)
		}
	})

	t.Run("auto degradation to partial_success on warnings", func(t *testing.T) {
		diag := agg.BuildDiagnosticsWithOptions(diagnostics.DiagnosticOptions{
			Status:    model.StatusSuccess,
			Warnings:  []string{"OCR failed on page 2", "OCR failed on page 2"}, // Includes duplicate
			StartTime: startTime,
		})

		if diag.Status != model.StatusPartialSuccess {
			t.Errorf("expected status %q, got %q", model.StatusPartialSuccess, diag.Status)
		}
		if len(diag.Warnings) != 1 {
			t.Errorf("expected 1 deduplicated warning, got %d (%v)", len(diag.Warnings), diag.Warnings)
		}
		if err := diag.Validate(); err != nil {
			t.Fatalf("diagnostics validation failed: %v", err)
		}
	})

	t.Run("auto degradation to partial_success on skipped pages", func(t *testing.T) {
		diag := agg.BuildDiagnosticsWithOptions(diagnostics.DiagnosticOptions{
			Status:       model.StatusSuccess,
			SkippedPages: []int{5, 2, 5}, // Includes unsorted duplicate
			RetryCount:   2,
			StartTime:    startTime,
		})

		if diag.Status != model.StatusPartialSuccess {
			t.Errorf("expected status %q, got %q", model.StatusPartialSuccess, diag.Status)
		}
		if len(diag.SkippedPages) != 2 || diag.SkippedPages[0] != 2 || diag.SkippedPages[1] != 5 {
			t.Errorf("expected sorted deduplicated skippedPages [2 5], got %v", diag.SkippedPages)
		}
		if diag.RetryCount != 2 {
			t.Errorf("expected retryCount 2, got %d", diag.RetryCount)
		}
		if err := diag.Validate(); err != nil {
			t.Fatalf("diagnostics validation failed: %v", err)
		}
	})

	t.Run("preserve failed status", func(t *testing.T) {
		diag := agg.BuildDiagnosticsWithOptions(diagnostics.DiagnosticOptions{
			Status:    model.StatusFailed,
			Errors:    []string{"ENCRYPTED_DOCUMENT: PDF is encrypted"},
			StartTime: startTime,
		})

		if diag.Status != model.StatusFailed {
			t.Errorf("expected status %q, got %q", model.StatusFailed, diag.Status)
		}
		if len(diag.Errors) != 1 {
			t.Errorf("expected 1 error, got %d", len(diag.Errors))
		}
		if err := diag.Validate(); err != nil {
			t.Fatalf("diagnostics validation failed: %v", err)
		}
	})
}

func TestValidatePDFStream(t *testing.T) {
	t.Run("nil reader returns ErrInvalidDocument", func(t *testing.T) {
		_, err := diagnostics.ValidatePDFStream(nil)
		if !errors.Is(err, model.ErrInvalidDocument) {
			t.Errorf("expected ErrInvalidDocument, got %v", err)
		}
	})

	t.Run("empty reader returns ErrInvalidDocument", func(t *testing.T) {
		_, err := diagnostics.ValidatePDFStream(strings.NewReader(""))
		if !errors.Is(err, model.ErrInvalidDocument) {
			t.Errorf("expected ErrInvalidDocument, got %v", err)
		}
	})

	t.Run("non-PDF content returns ErrInvalidDocument", func(t *testing.T) {
		_, err := diagnostics.ValidatePDFStream(strings.NewReader("Hello world, this is plain text!"))
		if !errors.Is(err, model.ErrInvalidDocument) {
			t.Errorf("expected ErrInvalidDocument, got %v", err)
		}
	})

	t.Run("valid PDF header passes validation", func(t *testing.T) {
		validPDF := "%PDF-1.4\n1 0 obj\n<< /Title (Test) >>\nendobj\n%%EOF"
		data, err := diagnostics.ValidatePDFStream(strings.NewReader(validPDF))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !bytes.Equal(data, []byte(validPDF)) {
			t.Errorf("expected returned bytes to match input")
		}
	})

	t.Run("encrypted PDF returns ErrEncryptedDocument", func(t *testing.T) {
		encryptedPDF := "%PDF-1.7\n1 0 obj\n<< /Filter /Standard /Encrypt 2 0 R >>\nendobj\n%%EOF"
		_, err := diagnostics.ValidatePDFStream(strings.NewReader(encryptedPDF))
		if !errors.Is(err, model.ErrEncryptedDocument) {
			t.Errorf("expected ErrEncryptedDocument, got %v", err)
		}
	})
}

func TestMapFirecrawlError(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		if err := diagnostics.MapFirecrawlError(nil); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("maps encrypted error strings to ErrEncryptedDocument", func(t *testing.T) {
		err := diagnostics.MapFirecrawlError(errors.New("HTTP 400: document is encrypted with user password"))
		if !errors.Is(err, model.ErrEncryptedDocument) {
			t.Errorf("expected ErrEncryptedDocument, got %v", err)
		}
	})

	t.Run("maps invalid pdf error strings to ErrInvalidDocument", func(t *testing.T) {
		err := diagnostics.MapFirecrawlError(errors.New("HTTP 422: malformed or corrupted PDF payload"))
		if !errors.Is(err, model.ErrInvalidDocument) {
			t.Errorf("expected ErrInvalidDocument, got %v", err)
		}
	})
}

func TestExtractSkippedPagesAndRetryCount(t *testing.T) {
	raw := &model.RawExtractionResult{
		Metadata: map[string]interface{}{
			"skipped_pages": []interface{}{float64(3), float64(1)},
			"retry_count":   float64(2),
		},
		JSONLayout: map[string]interface{}{
			"failed_pages": []interface{}{float64(3), float64(5)},
		},
	}

	pages := diagnostics.ExtractSkippedPages(raw)
	if len(pages) != 3 || pages[0] != 1 || pages[1] != 3 || pages[2] != 5 {
		t.Errorf("expected extracted skipped pages [1 3 5], got %v", pages)
	}

	retries := diagnostics.ExtractRetryCount(raw)
	if retries != 2 {
		t.Errorf("expected retry count 2, got %d", retries)
	}
}
