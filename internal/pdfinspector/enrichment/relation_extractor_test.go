package enrichment_test

import (
	"context"
	"testing"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestRuleBasedRelationExtractor(t *testing.T) {
	ctx := context.Background()
	extractor := enrichment.NewRuleBasedRelationExtractor()

	t.Run("empty input returns empty relation list", func(t *testing.T) {
		relations, err := extractor.ExtractRelations(ctx, enrichment.RelationInput{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(relations) != 0 {
			t.Errorf("expected 0 relations, got %d", len(relations))
		}
	})

	t.Run("extracts Entity-Entity and Entity-Concept SPO relations with deterministic IDs", func(t *testing.T) {
		input := enrichment.RelationInput{
			Chunks: []pdfmodel.KnowledgeChunk{
				{
					ChunkID:         "chunk-1",
					ContentMarkdown: "Rick Rubin founded Def Jam Recordings in New York to pioneer Vector Search Optimization.",
					Entities: []pdfmodel.EntityMention{
						{Text: "Rick Rubin", Type: pdfmodel.EntityTypePerson, ChunkID: "chunk-1"},
						{Text: "Def Jam Recordings", Type: pdfmodel.EntityTypeOrganization, ChunkID: "chunk-1"},
					},
				},
			},
			Entities: []pdfmodel.Entity{
				{ID: "entity:rick-rubin", Name: "Rick Rubin", Type: pdfmodel.EntityTypePerson},
				{ID: "entity:def-jam-recordings", Name: "Def Jam Recordings", Type: pdfmodel.EntityTypeOrganization},
			},
			Concepts: []pdfmodel.Concept{
				{ID: "concept:vector-search-optimization", Name: "Vector Search Optimization", Score: 0.95},
			},
		}

		relations, err := extractor.ExtractRelations(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(relations) == 0 {
			t.Fatal("expected relations to be extracted")
		}

		foundFoundedBy := false
		for _, rel := range relations {
			if rel.SubjectID == "entity:rick-rubin" && rel.ObjectID == "entity:def-jam-recordings" {
				foundFoundedBy = true
				if rel.Predicate != pdfmodel.RelationTypeFoundedBy {
					t.Errorf("expected RelationTypeFoundedBy, got %q", rel.Predicate)
				}
				if rel.ID != "rel:entity:rick-rubin:founded_by:entity:def-jam-recordings" {
					t.Errorf("unexpected deterministic relation ID: %q", rel.ID)
				}
			}
		}

		if !foundFoundedBy {
			t.Error("expected 'Rick Rubin founded Def Jam Recordings' SPO relation")
		}
	})
}
