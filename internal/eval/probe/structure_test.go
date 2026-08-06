package probe

import (
	"context"
	"testing"

	indexingmodel "arca/internal/indexing/model"
	retrievalseam "arca/internal/retrieval/seam"
)

func TestStructureRerankerHeadingOverlapBonus(t *testing.T) {
	// Query tokens {tuning, in?} -> stopwords drop "in"; {tuning}.
	// Candidate A is "Tuning In" section: overlap 1.0; candidate B is an
	// unrelated section: overlap 0. Content scores favor B slightly; the
	// bonus must promote A above B.
	r := &StructureReranker{BonusAlpha: 0.5}
	ordered, err := r.Rerank(context.Background(), "What does the book say about tuning in?", []retrievalseam.SearchResult{
		{ChunkID: "b", Score: 0.80, Metadata: indexingmodel.VectorMetadata{SectionPath: "Quantum Theory"}},
		{ChunkID: "a", Score: 0.79, Metadata: indexingmodel.VectorMetadata{SectionPath: "Tuning In"}},
	})
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(ordered) != 2 || ordered[0].ChunkID != "a" {
		t.Fatalf("ordering = %+v, want [a b] (heading overlap bonus must promote a)", ordered)
	}
	// a: 0.79 * (1 + 0.5*1.0) = 1.185; b: 0.80 * 1.0 = 0.80
	if d := ordered[0].Score - 1.185; d > 0.0001 || d < -0.0001 {
		t.Fatalf("a score = %v, want ~1.185", ordered[0].Score)
	}
}

func TestStructureRerankerIdentityWhenNoOverlap(t *testing.T) {
	r := &StructureReranker{BonusAlpha: 0.5}
	ordered, err := r.Rerank(context.Background(), "leverage points in systems", []retrievalseam.SearchResult{
		{ChunkID: "x", Score: 0.9, Metadata: indexingmodel.VectorMetadata{SectionPath: "Random Section"}},
		{ChunkID: "y", Score: 0.8, Metadata: indexingmodel.VectorMetadata{SectionPath: "Another"}},
	})
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if ordered[0].ChunkID != "x" {
		t.Fatalf("ordering = %+v, want [x y] (content order preserved)", ordered)
	}
}

func TestStructureRerankerZeroAlphaIsIdentity(t *testing.T) {
	r := &StructureReranker{BonusAlpha: 0}
	ordered, err := r.Rerank(context.Background(), "tuning in", []retrievalseam.SearchResult{
		{ChunkID: "b", Score: 0.9, Metadata: indexingmodel.VectorMetadata{SectionPath: "Tuning In"}},
		{ChunkID: "a", Score: 0.95, Metadata: indexingmodel.VectorMetadata{SectionPath: "Tuning In"}},
	})
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if ordered[0].ChunkID != "a" {
		t.Fatalf("zero alpha must keep content ordering, got %+v", ordered)
	}
}

func TestStructureTokensDropsStopwords(t *testing.T) {
	got := structureTokens("What does the book say about one-stock systems?")
	want := []string{"one", "stock", "systems"}
	if len(got) != len(want) {
		t.Fatalf("tokens = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tokens = %v, want %v", got, want)
		}
	}
}
