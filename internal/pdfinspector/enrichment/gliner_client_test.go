package enrichment_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestGLiNEREntityExtractor(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully extracts entity mentions from GLiNER microservice endpoint", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"text":       "Russell Simmons",
					"label":      "person",
					"confidence": 0.96,
					"start":      0,
					"end":        15,
				},
			})
		}))
		defer ts.Close()

		extractor := enrichment.NewGLiNEREntityExtractor(ts.URL, 2*time.Second)

		chunks := []pdfmodel.KnowledgeChunk{
			{ChunkID: "chunk-1", ContentMarkdown: "Russell Simmons co-founded Def Jam."},
		}

		mentions, err := extractor.ExtractEntities(ctx, enrichment.EntityInput{
			Chunks:   chunks,
			Language: "en",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mentions) != 1 {
			t.Fatalf("expected 1 mention, got %d", len(mentions))
		}

		if mentions[0].Text != "Russell Simmons" || mentions[0].Type != pdfmodel.EntityTypePerson {
			t.Errorf("unexpected mention: %+v", mentions[0])
		}
	})

	t.Run("falls back to RuleBasedEntityExtractor on HTTP service failure", func(t *testing.T) {
		extractor := enrichment.NewGLiNEREntityExtractor("http://localhost:9999/unreachable", 100*time.Millisecond)

		chunks := []pdfmodel.KnowledgeChunk{
			{ChunkID: "chunk-1", ContentMarkdown: "Rick Rubin founded Def Jam Recordings in New York."},
		}

		mentions, err := extractor.ExtractEntities(ctx, enrichment.EntityInput{
			Chunks:   chunks,
			Language: "en",
		})
		if err != nil {
			t.Fatalf("unexpected error during fallback: %v", err)
		}

		if len(mentions) == 0 {
			t.Fatal("expected fallback rule-based mentions, got 0")
		}
	})
}
