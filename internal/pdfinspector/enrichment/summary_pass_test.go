package enrichment_test

import (
	"context"
	"testing"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestSummaryPass(t *testing.T) {
	ctx := context.Background()
	pass := enrichment.NewSummaryPass(nil)

	t.Run("attaches document summary and chunk summaries to input models", func(t *testing.T) {
		input := &enrichment.EnrichmentInput{
			Metadata: &pdfmodel.DocumentMetadata{
				Keywords: []pdfmodel.Keyword{
					{Value: "vector search", Score: 0.9},
				},
				Concepts: []pdfmodel.Concept{
					{Name: "Vector Search Optimization", Score: 0.95},
				},
			},
			Chunks: []pdfmodel.KnowledgeChunk{
				{
					ChunkID:         "chunk-1",
					ContentMarkdown: "We optimize vector search indices for high performance.",
				},
			},
		}

		err := pass.Execute(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if input.Metadata.Summary == nil || input.Metadata.Summary.Text == "" {
			t.Error("expected document metadata to contain summary")
		}

		if input.Chunks[0].Summary == nil || input.Chunks[0].Summary.Text == "" {
			t.Error("expected chunk 1 to contain summary")
		}
	})
}
