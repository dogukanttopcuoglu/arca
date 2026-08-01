package inspector_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"arca/internal/pdfinspector/assets"
	"arca/internal/pdfinspector/chunking"
	"arca/internal/pdfinspector/config"
	"arca/internal/pdfinspector/diagnostics"
	"arca/internal/pdfinspector/firecrawl"
	"arca/internal/pdfinspector/inspector"
	"arca/internal/pdfinspector/semantic"
)

func TestPDFInspector_Bootstrap(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"markdown": "# Sample Extracted Document\n\nThis is a bootstrap extraction.",
			"json_layout": {},
			"metadata": {},
			"ocr_applied": false
		}`))
	}))
	defer ts.Close()

	cfg := config.LoadFromEnv()
	cfg.FirecrawlBaseURL = ts.URL
	client := firecrawl.NewHTTPClient(cfg.FirecrawlBaseURL)
	proc := semantic.NewProcessor()
	chunker := chunking.NewEngine()
	ext := assets.NewExtractor()
	agg := diagnostics.NewAggregator()

	insp := inspector.NewPDFInspector(cfg, client, proc, chunker, ext, agg)

	dummyPDF := strings.NewReader("%PDF-1.4 Header Sample PDF Content")
	result, err := insp.InspectPDF(context.Background(), dummyPDF)
	if err != nil {
		t.Fatalf("expected no error during bootstrap test, got: %v", err)
	}

	if result.Diagnostics.Status != "success" {
		t.Errorf("expected status 'success', got '%s'", result.Diagnostics.Status)
	}

	if len(result.Chunks) == 0 {
		t.Errorf("expected non-empty chunks list in bootstrap result")
	}

	if result.Content.Markdown == "" {
		t.Errorf("expected non-empty markdown content")
	}

	if err := result.Validate(); err != nil {
		t.Errorf("expected result to pass Validate(), got: %v", err)
	}
}

func TestPDFInspector_E2E_MultiPageDocument(t *testing.T) {
	multiPagePayload := "{\n" +
		`"markdown": "# Architecture Overview\n\nThis document describes ARC system architecture.\n\n## Data Layer\n\n| Table | Type |\n| --- | --- |\n| Postgres | RDBMS |\n\n` + "```go\\nfunc main() {}\\n```" + `\n\n$$E = mc^2$$\n\nAs described by Smith et al. [1].",` + "\n" +
		`"json_layout": {` + "\n" +
		`	"pages": [` + "\n" +
		`		{"page_number": 1, "markdown": "# Architecture Overview\n\nThis document describes ARC system architecture."},` + "\n" +
		`		{"page_number": 2, "markdown": "## Data Layer\n\n| Table | Type |\n| --- | --- |\n| Postgres | RDBMS |\n\n` + "```go\\nfunc main() {}\\n```" + `\n\n$$E = mc^2$$\n\nAs described by Smith et al. [1]."}` + "\n" +
		`	]` + "\n" +
		`},` + "\n" +
		`"metadata": {` + "\n" +
		`	"title": "ARC Architecture Spec",` + "\n" +
		`	"author": "ARC Team",` + "\n" +
		`	"page_count": 2,` + "\n" +
		`	"fonts": ["Inter", "Fira Code"],` + "\n" +
		`	"searchable": true` + "\n" +
		`},` + "\n" +
		`"ocr_applied": false` + "\n" +
		"}"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(multiPagePayload))
	}))
	defer ts.Close()

	cfg := config.LoadFromEnv()
	cfg.FirecrawlBaseURL = ts.URL
	client := firecrawl.NewHTTPClient(cfg.FirecrawlBaseURL)
	proc := semantic.NewProcessor()
	chunker := chunking.NewEngine()
	ext := assets.NewExtractor()
	agg := diagnostics.NewAggregator()

	insp := inspector.NewPDFInspector(cfg, client, proc, chunker, ext, agg)

	pdfStream := strings.NewReader("%PDF-1.5 Header Multi-page ARC document payload")
	result, err := insp.InspectPDF(context.Background(), pdfStream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Document.Title != "ARC Architecture Spec" {
		t.Errorf("expected Title 'ARC Architecture Spec', got %q", result.Document.Title)
	}

	if result.Document.Author != "ARC Team" {
		t.Errorf("expected Author 'ARC Team', got %q", result.Document.Author)
	}

	if result.Document.PageCount != 2 {
		t.Errorf("expected PageCount 2, got %d", result.Document.PageCount)
	}

	if len(result.Content.PageMap) != 2 {
		t.Errorf("expected PageMap length 2, got %d", len(result.Content.PageMap))
	}

	if len(result.SemanticTree.RootNodes) == 0 {
		t.Errorf("expected root nodes in semantic tree")
	}

	if len(result.Chunks) == 0 {
		t.Errorf("expected non-empty chunks")
	}

	if len(result.Assets.Tables) == 0 {
		t.Errorf("expected extracted tables")
	}

	if len(result.Assets.CodeBlocks) == 0 {
		t.Errorf("expected extracted code blocks")
	}

	if len(result.Assets.Equations) == 0 {
		t.Errorf("expected extracted equations")
	}

	if len(result.Assets.Citations) == 0 {
		t.Errorf("expected extracted citations")
	}

	if err := result.Validate(); err != nil {
		t.Fatalf("expected valid inspection result schema, got: %v", err)
	}
}

func TestPDFInspector_ContextCancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"markdown": "# Cancelled", "metadata": {}}`))
	}))
	defer ts.Close()

	cfg := config.LoadFromEnv()
	cfg.FirecrawlBaseURL = ts.URL
	client := firecrawl.NewHTTPClient(cfg.FirecrawlBaseURL)
	proc := semantic.NewProcessor()
	chunker := chunking.NewEngine()
	ext := assets.NewExtractor()
	agg := diagnostics.NewAggregator()

	insp := inspector.NewPDFInspector(cfg, client, proc, chunker, ext, agg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	dummyPDF := strings.NewReader("%PDF-1.4 Header test context cancellation")
	_, err := insp.InspectPDF(ctx, dummyPDF)
	if err == nil {
		t.Fatalf("expected error due to cancelled context, got nil")
	}
}
