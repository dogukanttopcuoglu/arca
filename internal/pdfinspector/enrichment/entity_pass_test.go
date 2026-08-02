package enrichment_test

import (
	"context"
	"testing"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestEntityExtractorPass(t *testing.T) {
	ctx := context.Background()
	pass := enrichment.NewEntityExtractorPass(nil)

	t.Run("attaches entity mentions to chunks and populates document-level entities via in-memory grouping", func(t *testing.T) {
		input := &enrichment.EnrichmentInput{
			Metadata: &pdfmodel.DocumentMetadata{
				Language: "en",
			},
			Chunks: []pdfmodel.KnowledgeChunk{
				{
					ChunkID:         "chunk-1",
					ContentMarkdown: "Rick Rubin founded Def Jam Recordings in New York.",
				},
				{
					ChunkID:         "chunk-2",
					ContentMarkdown: "Def Jam Recordings released many iconic albums in New York.",
				},
			},
		}

		err := pass.Execute(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(input.Chunks[0].Entities) == 0 {
			t.Error("expected entity mentions attached to Chunk 1")
		}

		if len(input.Metadata.Entities) == 0 {
			t.Error("expected document-level aggregated entities in DocumentMetadata")
		}

		foundDefJam := false
		for _, ent := range input.Metadata.Entities {
			if ent.Name == "Def Jam Recordings" || ent.Name == "Def Jam" {
				foundDefJam = true
				if len(ent.Mentions) < 2 {
					t.Errorf("expected 2 mentions grouped for Def Jam across chunks, got %d", len(ent.Mentions))
				}
			}
		}

		if !foundDefJam {
			t.Error("expected 'Def Jam Recordings' entity to be aggregated at document level")
		}
	})
}
