package main_test

import (
	"context"
	"io"
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

// mockLLMResponse returns a minimal OpenAI-compatible chat/completions body
// whose content carries a valid [Ref 1] marker.
func mockLLMResponse() string {
	return `{
  "choices": [{"message": {"role": "assistant", "content": "Knowledge systems transform raw documents into connected semantic graphs [Ref 1]."}}],
  "usage": {"prompt_tokens": 12, "completion_tokens": 18, "total_tokens": 30}
}`
}

// mockEvidenceGateResponse returns a structured gate decision the fake
// gateway serves for evidence-gate requests.
func mockEvidenceGateResponse() string {
	return `{
  "choices": [{"message": {"role": "assistant", "content": "{\"decision\":\"supported\"}"}}],
  "usage": {"prompt_tokens": 8, "completion_tokens": 4, "total_tokens": 12}
}`
}

// isEvidenceGateRequest reports whether the request body targets the
// pre-generation evidence gate (detected by its system instruction).
func isEvidenceGateRequest(body []byte) bool {
	return strings.Contains(string(body), "evidence gate")
}

// newTestApp wires the composition root against a mock Firecrawl service and a
// mock OpenAI-compatible LLM gateway, returning the app, the PDF path, and a
// cleanup function.
func newTestApp(t *testing.T) (*arccli.App, string, func()) {
	t.Helper()

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(mockExtractionJSON()))
	}))

	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		body, _ := io.ReadAll(r.Body)
		if isEvidenceGateRequest(body) {
			_, _ = w.Write([]byte(mockEvidenceGateResponse()))
			return
		}
		_, _ = w.Write([]byte(mockLLMResponse()))
	}))

	cfg := arccli.DefaultConfig()
	cfg.FirecrawlBaseURL = mockServer.URL
	cfg.LLMBaseURL = llmServer.URL
	cfg.LLMModel = "test-model"
	cfg.LLMProviderLabel = "test-gateway"
	// The M4 min-score (0.6) is calibrated on real nomic embeddings; the mock
	// embedding provider produces near-zero cosine similarities, so the
	// fixture explicitly disables the threshold to test rendering behavior.
	cfg.RetrievalMinScore = 0

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
		llmServer.Close()
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

	t.Run("executes 'ask' CLI command and renders the generated answer", func(t *testing.T) {
		out, err := app.RunAsk(ctx, "What is a knowledge system?")
		if err != nil {
			t.Fatalf("unexpected error running CLI ask: %v", err)
		}
		if out == "" {
			t.Error("expected non-empty output from CLI ask")
		}
		if !strings.Contains(out, "A:") {
			t.Errorf("expected ask output to render an answer, got: %s", out)
		}
		if !strings.Contains(out, "Sources:") {
			t.Errorf("expected ask output to include a Sources section, got: %s", out)
		}
	})

	t.Run("ask renders answer with citations and section metadata", func(t *testing.T) {
		out, err := app.RunAsk(ctx, "semantic boundaries")
		if err != nil {
			t.Fatalf("unexpected error running CLI ask: %v", err)
		}
		if !strings.Contains(out, "[Ref 1]") {
			t.Errorf("expected ask output to include the citation marker, got: %s", out)
		}
		if !strings.Contains(out, "document:") {
			t.Errorf("expected ask output to include document identity, got: %s", out)
		}
		if !strings.Contains(out, "section:") {
			t.Errorf("expected ask output to include section metadata, got: %s", out)
		}
		if !strings.Contains(out, "page(s)") {
			t.Errorf("expected ask output to include page numbers, got: %s", out)
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
