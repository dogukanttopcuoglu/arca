package enrichment_test

import (
	"context"
	"testing"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestRelationExtractorPass(t *testing.T) {
	ctx := context.Background()
	pass := enrichment.NewRelationExtractorPass(nil)

	t.Run("populates document-level relation catalog and chunk-specific relations", func(t *testing.T) {
		input := &enrichment.EnrichmentInput{
			Metadata: &pdfmodel.DocumentMetadata{
				Entities: []pdfmodel.Entity{
					{ID: "entity:rick-rubin", Name: "Rick Rubin", Type: pdfmodel.EntityTypePerson},
					{ID: "entity:def-jam-recordings", Name: "Def Jam Recordings", Type: pdfmodel.EntityTypeOrganization},
				},
				Concepts: []pdfmodel.Concept{
					{ID: "concept:vector-search", Name: "Vector Search", Score: 0.9},
				},
			},
			Chunks: []pdfmodel.KnowledgeChunk{
				{
					ChunkID:         "chunk-1",
					ContentMarkdown: "Rick Rubin founded Def Jam Recordings in New York for Vector Search.",
					Entities: []pdfmodel.EntityMention{
						{Text: "Rick Rubin", Type: pdfmodel.EntityTypePerson, ChunkID: "chunk-1"},
						{Text: "Def Jam Recordings", Type: pdfmodel.EntityTypeOrganization, ChunkID: "chunk-1"},
					},
				},
			},
		}

		_, err := pass.Execute(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(input.Metadata.Relations) == 0 {
			t.Error("expected document metadata to contain relations catalog")
		}

		if len(input.Chunks[0].Relations) == 0 {
			t.Error("expected chunk 1 to contain chunk-level relations")
		}
	})
}
