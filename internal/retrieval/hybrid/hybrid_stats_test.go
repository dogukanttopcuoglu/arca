package hybrid_test

import (
	"context"
	"testing"

	indexingmodel "arca/internal/indexing/model"
	"arca/internal/retrieval/hybrid"
	"arca/internal/retrieval/seam"
)

// staticRetriever returns a canned result list; used to compose deterministic
// dense/sparse streams for fusion tests.
type staticRetriever struct {
	results []seam.SearchResult
}

func (s staticRetriever) Retrieve(ctx context.Context, q seam.RetrievalQuery) ([]seam.SearchResult, error) {
	return s.results, nil
}

func result(chunkID string) seam.SearchResult {
	return seam.SearchResult{
		ChunkID:  chunkID,
		Metadata: indexingmodel.VectorMetadata{ChunkID: chunkID},
	}
}

func TestHybridRetriever_DeterministicTieBreak(t *testing.T) {
	ctx := context.Background()

	// Both streams return disjoint sets so each chunk appears exactly once
	// with RRF score 1/(60+rank) — ties between the two streams' members.
	dense := staticRetriever{results: []seam.SearchResult{result("chk-b"), result("chk-d")}}
	sparse := staticRetriever{results: []seam.SearchResult{result("chk-a"), result("chk-c")}}

	h := hybrid.NewHybridRetriever(dense, sparse)

	r1, err := h.Retrieve(ctx, seam.RetrievalQuery{QueryText: "q", TopK: 10, Mode: seam.RetrievalHybrid})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r2, err := h.Retrieve(ctx, seam.RetrievalQuery{QueryText: "q", TopK: 10, Mode: seam.RetrievalHybrid})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// RRF: chk-a and chk-b both score 1/61; chk-c and chk-d both 1/62.
	// Tie-break must be ascending ChunkID, deterministic across runs.
	ids := func(rs []seam.SearchResult) []string {
		out := make([]string, len(rs))
		for i, r := range rs {
			out[i] = r.ChunkID
		}
		return out
	}
	want := []string{"chk-a", "chk-b", "chk-c", "chk-d"}
	got1 := ids(r1)
	got2 := ids(r2)
	if len(got1) != 4 || len(got2) != 4 {
		t.Fatalf("expected 4 fused results, got %d and %d", len(got1), len(got2))
	}
	for i := range want {
		if got1[i] != want[i] || got2[i] != want[i] {
			t.Errorf("tied ordering must be ChunkID-ascending and deterministic: run1=%v run2=%v", got1, got2)
		}
	}
}

func TestHybridRetriever_PopulatesStreamStats(t *testing.T) {
	ctx := context.Background()

	dense := staticRetriever{results: []seam.SearchResult{result("chk-1"), result("chk-2")}}
	sparse := staticRetriever{results: []seam.SearchResult{result("chk-2"), result("chk-3")}}
	h := hybrid.NewHybridRetriever(dense, sparse)

	stats := &seam.RetrievalStats{}
	results, err := h.Retrieve(ctx, seam.RetrievalQuery{
		QueryText: "q",
		TopK:      10,
		Mode:      seam.RetrievalHybrid,
		MinScore:  0.5,
		Stats:     stats,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.DenseCandidates != 2 {
		t.Errorf("expected 2 dense candidates, got %d", stats.DenseCandidates)
	}
	if stats.SparseCandidates != 2 {
		t.Errorf("expected 2 sparse candidates, got %d", stats.SparseCandidates)
	}
	if stats.FusedCandidates != len(results) || stats.TopKReturned != len(results) {
		t.Errorf("expected fused candidates %d and returned %d, got %d/%d",
			len(results), len(results), stats.FusedCandidates, stats.TopKReturned)
	}
	if stats.TopKRequested != 10 || stats.MinScore != 0.5 {
		t.Errorf("expected TopK 10 and MinScore 0.5 recorded, got %+v", stats)
	}
}

func TestHybridRetriever_AbstainsWhenStreamsEmpty(t *testing.T) {
	ctx := context.Background()
	h := hybrid.NewHybridRetriever(staticRetriever{}, staticRetriever{})

	stats := &seam.RetrievalStats{}
	results, err := h.Retrieve(ctx, seam.RetrievalQuery{QueryText: "q", TopK: 5, Mode: seam.RetrievalHybrid, Stats: stats})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no fused results, got %d", len(results))
	}
	if stats.FusedCandidates != 0 {
		t.Errorf("expected 0 fused candidates, got %d", stats.FusedCandidates)
	}
}
