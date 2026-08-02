package enrichment_test

import (
	"context"
	"testing"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestConceptExtractorPass(t *testing.T) {
	ctx := context.Background()
	pass := enrichment.NewConceptExtractorPass(nil)

	t.Run("attaches concepts to document metadata and matching chunks", func(t *testing.T) {
		input := &enrichment.EnrichmentInput{
			Metadata: &pdfmodel.DocumentMetadata{
				Language: "en",
				Keywords: []pdfmodel.Keyword{
					{Value: "vector search", Score: 0.95},
				},
			},
			Tree: &pdfmodel.SemanticTree{
				RootNodes: []pdfmodel.SemanticNode{
					{
						ID:      "node-1",
						Heading: "Vector Search Optimization",
						Level:   1,
					},
				},
			},
			Chunks: []pdfmodel.KnowledgeChunk{
				{
					ChunkID:         "chunk-1",
					SectionPath:     "Vector Search Optimization",
					ContentMarkdown: "We optimize vector search indices for high performance.",
				},
			},
		}

		err := pass.Execute(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(input.Metadata.Concepts) == 0 {
			t.Error("expected document metadata to contain concepts")
		}

		if len(input.Chunks[0].Concepts) == 0 {
			t.Error("expected chunk 1 to contain attached concepts")
		}
	})
}
