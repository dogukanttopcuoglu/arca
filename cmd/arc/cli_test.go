package main_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	arccli "arca/cmd/arc/cli"
)

// mockExtractionJSON returns a minimal valid Firecrawl /v1/extract response body.
func mockExtractionJSON() string {
	return `{
  "markdown": "# Introduction to Knowledge Systems\n\nKnowledge systems transform raw documents into connected semantic graphs.\n\n## Section 1: Overview\n\nAccording to Smith et al. [1], semantic boundaries preserve context.\n\n[1] Smith, J. et al. (2025). Knowledge Ingestion & Semantic Preservation.",
  "json_layout": {
    "pages": [
      {
        "page_number": 1,
        "markdown": "# Introduction to Knowledge Systems\n\nKnowledge systems transform raw documents into connected semantic graphs."
      },
      {
        "page_number": 2,
        "markdown": "## Section 1: Overview\n\nAccording to Smith et al. [1], semantic boundaries preserve context."
      }
    ]
  },
  "metadata": {
    "title": "Introduction to Knowledge Systems",
    "author": "ARC Engineering",
    "page_count": 2,
    "searchable": true
  },
  "ocr_applied": false
}`
}

// newTestApp wires the composition root against a mock Firecrawl service and a
// temporary PDF file, returning the app, the PDF path, and a cleanup function.
func newTestApp(t *testing.T) (*arccli.App, string, func()) {
	t.Helper()

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockExtractionJSON()))
	}))

	cfg := arccli.DefaultConfig()
	cfg.FirecrawlBaseURL = mockServer.URL

	runtime, err := arccli.NewRuntime(cfg)
	if err != nil {
		t.Fatalf("failed to construct runtime: %v", err)
	}

	// Minimal valid PDF header + extraction payload.
	pdfPath := filepath.Join(t.TempDir(), "sample.pdf")
	pdfContent := "%PDF-1.4 Header Sample PDF\n" + mockExtractionJSON()
	if err := os.WriteFile(pdfPath, []byte(pdfContent), 0644); err != nil {
		t.Fatalf("failed to write temp PDF: %v", err)
	}

	app := arccli.NewAppWithRuntime(runtime)
	cleanup := func() {
		mockServer.Close()
	}
	return app, pdfPath, cleanup
}

func TestCLIToolCommands(t *testing.T) {
	ctx := context.Background()
	app, pdfPath, cleanup := newTestApp(t)
	defer cleanup()

	t.Run("executes 'inspect' CLI command end to end", func(t *testing.T) {
		out, err := app.RunInspect(ctx, pdfPath)
		if err != nil {
			t.Fatalf("unexpected error running CLI inspect: %v", err)
		}
		if out == "" {
			t.Error("expected non-empty output from CLI inspect")
		}
		if !strings.Contains(out, "Indexed") {
			t.Errorf("expected inspect output to report indexing, got: %s", out)
		}
	})

	t.Run("executes 'ask' CLI command", func(t *testing.T) {
		out, err := app.RunAsk(ctx, "What is a knowledge system?")
		if err != nil {
			t.Fatalf("unexpected error running CLI ask: %v", err)
		}
		if out == "" {
			t.Error("expected non-empty output from CLI ask")
		}
		if !strings.Contains(out, "Found") {
			t.Errorf("expected ask output to report found chunks, got: %s", out)
		}
	})

	t.Run("ask returns chunks with metadata and citations", func(t *testing.T) {
		out, err := app.RunAsk(ctx, "semantic boundaries")
		if err != nil {
			t.Fatalf("unexpected error running CLI ask: %v", err)
		}
		if !strings.Contains(out, "citations=") {
			t.Errorf("expected ask output to include citations, got: %s", out)
		}
		if !strings.Contains(out, "Smith") {
			t.Errorf("expected ask output to include citation text, got: %s", out)
		}
		if !strings.Contains(out, "section=") {
			t.Errorf("expected ask output to include section metadata, got: %s", out)
		}
	})

	t.Run("executes 'research' CLI command", func(t *testing.T) {
		out, err := app.RunResearch(ctx, "Synthesize creative principles")
		if err != nil {
			t.Fatalf("unexpected error running CLI research: %v", err)
		}
		if out == "" {
			t.Error("expected non-empty output from CLI research")
		}
	})
}
