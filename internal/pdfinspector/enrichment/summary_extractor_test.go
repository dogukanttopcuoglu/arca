package enrichment_test

import (
	"context"
	"testing"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestRuleBasedSummaryExtractor(t *testing.T) {
	ctx := context.Background()
	extractor := enrichment.NewRuleBasedSummaryExtractor()

	t.Run("empty input returns empty summary result", func(t *testing.T) {
		res, err := extractor.ExtractSummaries(ctx, enrichment.SummaryInput{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.DocumentSummary != nil {
			t.Errorf("expected nil DocumentSummary for empty input, got %v", res.DocumentSummary)
		}
	})

	t.Run("extracts document and chunk summaries extractively", func(t *testing.T) {
		input := enrichment.SummaryInput{
			Chunks: []pdfmodel.KnowledgeChunk{
				{
					ChunkID:         "chunk-1",
					SectionPath:     "Vector Search",
					ContentMarkdown: "We optimize vector search indices for high performance. This enables sub-millisecond retrieval across large corpora.",
				},
			},
			Concepts: []pdfmodel.Concept{
				{Name: "Vector Search Optimization", Score: 0.95},
			},
			Entities: []pdfmodel.Entity{
				{Name: "Def Jam Recordings", Type: pdfmodel.EntityTypeOrganization},
			},
		}

		res, err := extractor.ExtractSummaries(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.DocumentSummary == nil || res.DocumentSummary.Text == "" {
			t.Fatal("expected non-empty DocumentSummary")
		}

		if res.DocumentSummary.Source != pdfmodel.SummarySourceRuleBased {
			t.Errorf("expected SummarySourceRuleBased, got %q", res.DocumentSummary.Source)
		}

		if chunkSum, ok := res.ChunkSummaries["chunk-1"]; !ok || chunkSum.Text == "" {
			t.Error("expected extractive summary for chunk-1")
		}
	})
}
