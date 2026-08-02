package enrichment_test

import (
	"context"
	"testing"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestRuleBasedConceptExtractor(t *testing.T) {
	ctx := context.Background()
	extractor := enrichment.NewRuleBasedConceptExtractor()

	t.Run("empty input returns empty concept list", func(t *testing.T) {
		concepts, err := extractor.ExtractConcepts(ctx, enrichment.ConceptInput{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(concepts) != 0 {
			t.Errorf("expected 0 concepts, got %d", len(concepts))
		}
	})

	t.Run("synthesizes concepts from semantic tree headings and keywords", func(t *testing.T) {
		input := enrichment.ConceptInput{
			Tree: &pdfmodel.SemanticTree{
				RootNodes: []pdfmodel.SemanticNode{
					{
						ID:      "node-1",
						Heading: "Vector Search Optimization",
						Level:   1,
					},
				},
			},
			Keywords: []pdfmodel.Keyword{
				{Value: "vector search", Score: 0.95},
				{Value: "indexing", Score: 0.80},
			},
			Language: "en",
		}

		concepts, err := extractor.ExtractConcepts(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(concepts) == 0 {
			t.Fatal("expected concepts to be extracted")
		}

		foundVectorSearch := false
		for _, c := range concepts {
			if c.Name == "Vector Search Optimization" || c.Name == "vector search" {
				foundVectorSearch = true
				if c.Source != pdfmodel.ConceptSourceRuleBased {
					t.Errorf("expected ConceptSourceRuleBased, got %q", c.Source)
				}
			}
		}

		if !foundVectorSearch {
			t.Error("expected concept 'Vector Search Optimization' or 'vector search' to be synthesized")
		}
	})
}
