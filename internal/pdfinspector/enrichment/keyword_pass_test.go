package enrichment_test

import (
	"context"
	"testing"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestKeywordExtractorPass(t *testing.T) {
	ctx := context.Background()
	pass := enrichment.NewKeywordExtractorPass(nil)

	t.Run("attaches extracted keywords to both document metadata and individual chunks", func(t *testing.T) {
		input := &enrichment.EnrichmentInput{
			Metadata: &pdfmodel.DocumentMetadata{
				Language: "en",
			},
			Chunks: []pdfmodel.KnowledgeChunk{
				{
					ChunkID:         "chunk-1",
					ContentMarkdown: "Database vector search optimization and indexing performance.",
				},
			},
		}

		_, err := pass.Execute(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(input.Metadata.Keywords) == 0 {
			t.Error("expected keywords attached to DocumentMetadata")
		}

		if len(input.Chunks[0].Keywords) == 0 {
			t.Error("expected keywords attached to individual KnowledgeChunk")
		}

		foundVector := false
		for _, kw := range input.Chunks[0].Keywords {
			if kw.Value == "vector" || kw.Value == "database" || kw.Value == "indexing" {
				foundVector = true
			}
		}
		if !foundVector {
			t.Error("expected chunk to contain extracted keywords")
		}
	})
}
