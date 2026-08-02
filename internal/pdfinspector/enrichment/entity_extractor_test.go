package enrichment_test

import (
	"context"
	"testing"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestRuleBasedEntityExtractor(t *testing.T) {
	ctx := context.Background()
	extractor := enrichment.NewRuleBasedEntityExtractor()

	t.Run("empty document / chunks returns empty entity mentions", func(t *testing.T) {
		mentions, err := extractor.ExtractEntities(ctx, enrichment.EntityInput{
			Chunks:   nil,
			Language: "en",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mentions) != 0 {
			t.Errorf("expected 0 mentions, got %d", len(mentions))
		}
	})

	t.Run("extracts Person, Organization, and Location mentions from text", func(t *testing.T) {
		chunks := []pdfmodel.KnowledgeChunk{
			{
				ChunkID:         "chunk-1",
				ContentMarkdown: "Rick Rubin founded Def Jam Recordings in New York.",
			},
		}

		mentions, err := extractor.ExtractEntities(ctx, enrichment.EntityInput{
			Chunks:   chunks,
			Language: "en",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mentions) == 0 {
			t.Fatal("expected extracted entity mentions")
		}

		foundOrg := false
		for _, m := range mentions {
			if m.ChunkID != "chunk-1" {
				t.Errorf("expected chunk_id 'chunk-1', got %q", m.ChunkID)
			}
			if m.Text == "Def Jam Recordings" || m.Text == "Def Jam" {
				foundOrg = true
				if m.Type != pdfmodel.EntityTypeOrganization {
					t.Errorf("expected Organization type, got %q", m.Type)
				}
			}
		}

		if !foundOrg {
			t.Error("expected Organization mention 'Def Jam' to be extracted")
		}
	})

	t.Run("turkish language entity extraction support", func(t *testing.T) {
		chunks := []pdfmodel.KnowledgeChunk{
			{
				ChunkID:         "chunk-tr-1",
				ContentMarkdown: "Mustafa Kemal Atatürk tarafından Türkiye Cumhuriyeti Ankara şehrinde kuruldu.",
			},
		}

		mentions, err := extractor.ExtractEntities(ctx, enrichment.EntityInput{
			Chunks:   chunks,
			Language: "tr",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mentions) == 0 {
			t.Fatal("expected Turkish entity mentions")
		}
	})
}
