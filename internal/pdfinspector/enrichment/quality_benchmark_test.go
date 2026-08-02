package enrichment_test

import (
	"strings"
	"testing"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestEmpiricalEnrichmentQualityBenchmark(t *testing.T) {
	enricher := enrichment.NewEnricher()

	meta := &pdfmodel.DocumentMetadata{
		Title:  "Extracted PDF Document",
		Author: "Firecrawl Inspector",
	}

	tree := &pdfmodel.SemanticTree{
		RootNodes: []pdfmodel.SemanticNode{
			{Heading: "The Creative Act: A Way of Being", Level: 1, PageNumbers: []int{1}},
			{Heading: "Beginner's Mind", Level: 2, PageNumbers: []int{71}},
		},
	}

	pageMap := []pdfmodel.PageMap{
		{PageNumber: 1, Markdown: "# The Creative Act: A Way of Being\nwritten by Rick Rubin"},
		{PageNumber: 71, Markdown: "## Beginner's Mind\nRick Rubin founded Def Jam Recordings in New York."},
	}

	chunks := []pdfmodel.KnowledgeChunk{
		{
			ChunkID:         "chunk-1",
			SectionPath:     "The Creative Act: A Way of Being > Beginner's Mind",
			HeadingLevel:    2,
			PageNumbers:     []int{71},
			ContentMarkdown: "Rick Rubin founded Def Jam Recordings in New York.",
		},
	}

	input := &enrichment.EnrichmentInput{
		Metadata: meta,
		Tree:     tree,
		PageMap:  pageMap,
		Chunks:   chunks,
		Filename: "rick-rubin.pdf",
	}

	report := enricher.Enrich(input)
	if report == nil {
		t.Fatal("expected non-nil EnrichmentReport")
	}

	// 1. Quality Benchmark A: Verify Concept Domain Boundaries (NO unigrams or location fragments like "york")
	for _, c := range input.Metadata.Concepts {
		if strings.ToLower(c.Name) == "york" || strings.ToLower(c.Name) == "def" {
			t.Errorf("QUALITY BENCHMARK FAILURE: Unigram concept leak detected in Metadata.Concepts: %q", c.Name)
		}
	}

	// 2. Quality Benchmark B: Verify Predicate Directionality (Def Jam -> founded_by -> Rick Rubin)
	foundFoundedBy := false
	for _, rel := range input.Metadata.Relations {
		if rel.Predicate == pdfmodel.RelationTypeFoundedBy {
			foundFoundedBy = true
			if !strings.Contains(rel.SubjectID, "def jam") || !strings.Contains(rel.ObjectID, "rick rubin") {
				t.Errorf("QUALITY BENCHMARK FAILURE: Predicate direction inverted. Subject: %q, Object: %q", rel.SubjectID, rel.ObjectID)
			}
		}
	}

	if !foundFoundedBy {
		t.Error("expected 'founded_by' relation in Metadata.Relations")
	}
}
