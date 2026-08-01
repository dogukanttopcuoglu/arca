package inspector_test

import (
	"context"
	"errors"
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
	"arca/internal/pdfinspector/model"
	"arca/internal/pdfinspector/semantic"
)

func setupInspector(serverURL string) *inspector.PDFInspector {
	cfg := config.LoadFromEnv()
	cfg.FirecrawlBaseURL = serverURL
	client := firecrawl.NewHTTPClient(cfg.FirecrawlBaseURL)
	proc := semantic.NewProcessor()
	chunker := chunking.NewEngine()
	ext := assets.NewExtractor()
	agg := diagnostics.NewAggregator()
	return inspector.NewPDFInspector(cfg, client, proc, chunker, ext, agg)
}

func TestPDFInspector_Resiliency_InvalidPDF(t *testing.T) {
	insp := setupInspector("http://localhost:9999")

	t.Run("empty stream returns ErrInvalidDocument", func(t *testing.T) {
		res, err := insp.InspectPDF(context.Background(), strings.NewReader(""))
		if !errors.Is(err, model.ErrInvalidDocument) {
			t.Errorf("expected ErrInvalidDocument, got %v", err)
		}
		if res == nil || res.Diagnostics.Status != model.StatusFailed {
			t.Errorf("expected diagnostics status 'failed', got %v", res)
		}
	})

	t.Run("non-PDF bytes return ErrInvalidDocument", func(t *testing.T) {
		res, err := insp.InspectPDF(context.Background(), strings.NewReader("Corrupted data string without PDF header"))
		if !errors.Is(err, model.ErrInvalidDocument) {
			t.Errorf("expected ErrInvalidDocument, got %v", err)
		}
		if res == nil || res.Diagnostics.Status != model.StatusFailed {
			t.Errorf("expected diagnostics status 'failed', got %v", res)
		}
	})
}

func TestPDFInspector_Resiliency_EncryptedPDF(t *testing.T) {
	insp := setupInspector("http://localhost:9999")

	encryptedPayload := "%PDF-1.7\n1 0 obj\n<< /Filter /Standard /Encrypt 2 0 R >>\nendobj\n%%EOF"
	res, err := insp.InspectPDF(context.Background(), strings.NewReader(encryptedPayload))

	if !errors.Is(err, model.ErrEncryptedDocument) {
		t.Errorf("expected ErrEncryptedDocument, got %v", err)
	}
	if res == nil || res.Diagnostics.Status != model.StatusFailed {
		t.Errorf("expected diagnostics status 'failed', got %v", res)
	}
	if len(res.Diagnostics.Errors) == 0 || !strings.Contains(res.Diagnostics.Errors[0], "ENCRYPTED_DOCUMENT") {
		t.Errorf("expected error list to contain ENCRYPTED_DOCUMENT, got %v", res.Diagnostics.Errors)
	}
}

func TestPDFInspector_Resiliency_FirecrawlHTTPErrorMapping(t *testing.T) {
	t.Run("map encrypted HTTP error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": "client error (status 400): document is encrypted with password"}`))
		}))
		defer ts.Close()

		insp := setupInspector(ts.URL)
		validHeaderPDF := "%PDF-1.4\nSample PDF payload"
		res, err := insp.InspectPDF(context.Background(), strings.NewReader(validHeaderPDF))

		if !errors.Is(err, model.ErrEncryptedDocument) {
			t.Errorf("expected ErrEncryptedDocument mapped from HTTP response, got %v", err)
		}
		if res.Diagnostics.Status != model.StatusFailed {
			t.Errorf("expected status 'failed', got %q", res.Diagnostics.Status)
		}
	})

	t.Run("map invalid pdf HTTP error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error": "client error (status 422): malformed or corrupted pdf structure"}`))
		}))
		defer ts.Close()

		insp := setupInspector(ts.URL)
		validHeaderPDF := "%PDF-1.4\nSample PDF payload"
		res, err := insp.InspectPDF(context.Background(), strings.NewReader(validHeaderPDF))

		if !errors.Is(err, model.ErrInvalidDocument) {
			t.Errorf("expected ErrInvalidDocument mapped from HTTP response, got %v", err)
		}
		if res.Diagnostics.Status != model.StatusFailed {
			t.Errorf("expected status 'failed', got %q", res.Diagnostics.Status)
		}
	})
}

func TestPDFInspector_Resiliency_PartialSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"markdown": "# Page 1 Section\n\nContent for page 1.\n\n# Page 3 Section\n\nContent for page 3.",
			"json_layout": {},
			"metadata": {
				"skipped_pages": [2],
				"retry_count": 1
			},
			"ocr_applied": true
		}`))
	}))
	defer ts.Close()

	insp := setupInspector(ts.URL)
	validPDF := "%PDF-1.4\nValid multi-page document payload"

	res, err := insp.InspectPDF(context.Background(), strings.NewReader(validPDF))
	if err != nil {
		t.Fatalf("expected no error during partial success test, got: %v", err)
	}

	if res.Diagnostics.Status != model.StatusPartialSuccess {
		t.Errorf("expected status %q, got %q", model.StatusPartialSuccess, res.Diagnostics.Status)
	}

	if len(res.Diagnostics.SkippedPages) != 1 || res.Diagnostics.SkippedPages[0] != 2 {
		t.Errorf("expected skippedPages [2], got %v", res.Diagnostics.SkippedPages)
	}

	if res.Diagnostics.RetryCount != 1 {
		t.Errorf("expected retryCount 1, got %d", res.Diagnostics.RetryCount)
	}

	if res.Diagnostics.ProcessingTimeMs < 0 {
		t.Errorf("expected processingTimeMs >= 0, got %d", res.Diagnostics.ProcessingTimeMs)
	}

	if err := res.Validate(); err != nil {
		t.Fatalf("expected valid PDFInspectionResult schema, got: %v", err)
	}
}
