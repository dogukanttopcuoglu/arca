package enrichment_test

import (
	"context"
	"strings"
	"testing"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

// Behavior Contract 1: Entity Extraction must identify compound entity phrases like Def Jam Recordings
func TestEntityExtraction_DefJamIsSingleEntity(t *testing.T) {
	ctx := context.Background()
	extractor := enrichment.NewRuleBasedEntityExtractor()

	chunks := []pdfmodel.KnowledgeChunk{
		{
			ChunkID:         "chunk-test-1",
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

	foundDefJam := false
	for _, m := range mentions {
		if m.Text == "Def Jam Recordings" && m.Type == pdfmodel.EntityTypeOrganization {
			foundDefJam = true
		}
	}

	if !foundDefJam {
		t.Error("BEHAVIOR CONTRACT FAILURE: 'Def Jam Recordings' organization entity mention was not extracted")
	}
}

// Behavior Contract 2: Concept Extractor MUST reject single-word unigrams and location/entity fragments like "york" or "def"
func TestConceptExtractorRejectsEntityFragments(t *testing.T) {
	ctx := context.Background()
	extractor := enrichment.NewRuleBasedConceptExtractor()

	input := enrichment.ConceptInput{
		Tree: &pdfmodel.SemanticTree{
			RootNodes: []pdfmodel.SemanticNode{
				{Heading: "Beginner's Mind", Level: 1},
			},
		},
		Keywords: []pdfmodel.Keyword{
			{Value: "york", Score: 0.95},
			{Value: "def", Score: 0.95},
			{Value: "vector search", Score: 0.85},
		},
		Entities: []pdfmodel.Entity{
			{ID: "location:new-york", Name: "New York", Type: pdfmodel.EntityTypeLocation},
			{ID: "organization:def-jam-recordings", Name: "Def Jam Recordings", Type: pdfmodel.EntityTypeOrganization},
		},
		Language: "en",
	}

	concepts, err := extractor.ExtractConcepts(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, c := range concepts {
		lowerName := strings.ToLower(c.Name)
		if lowerName == "york" || lowerName == "def" {
			t.Errorf("BEHAVIOR CONTRACT VIOLATION: Unigram entity fragment %q was accepted as a concept!", c.Name)
		}
	}
}
