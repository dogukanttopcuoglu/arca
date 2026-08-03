package hybrid_test

import (
	"context"
	"testing"

	"arca/internal/retrieval/hybrid"
	"arca/internal/retrieval/seam"
)

// policyRetriever returns a canned stream; used to feed the hybrid retriever.
type policyRetriever struct {
	results []seam.SearchResult
}

func (p policyRetriever) Retrieve(ctx context.Context, q seam.RetrievalQuery) ([]seam.SearchResult, error) {
	return p.results, nil
}

func TestFusionPolicy_WeightedRRF(t *testing.T) {
	ctx := context.Background()

	dense := policyRetriever{results: []seam.SearchResult{result("chk-d1")}}
	sparse := policyRetriever{results: []seam.SearchResult{result("chk-s1")}}
	// Balanced: both rank 1 -> tie 1/61 each, ChunkID tie-break.
	hBalanced := hybrid.NewHybridRetriever(dense, sparse)
	rBal, err := hBalanced.Retrieve(ctx, seam.RetrievalQuery{QueryText: "q", TopK: 10, Mode: seam.RetrievalHybrid})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rBal) != 2 || rBal[0].Score != rBal[1].Score {
		t.Fatalf("expected balanced tie, got %+v", rBal)
	}

	// DenseBiased: sparse weight 0.25 -> chk-d1 (1/61) beats chk-s1 (0.25/61).
	hBiased := hybrid.NewHybridRetriever(dense, sparse,
		hybrid.WithFusionPolicy(hybrid.FusionPolicy{
			DenseWeight:  1.0,
			SparseWeight: 0.25,
			SparseCap:    0,
			RRFK:         60,
		}))
	rBias, err := hBiased.Retrieve(ctx, seam.RetrievalQuery{QueryText: "q", TopK: 10, Mode: seam.RetrievalHybrid})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rBias) != 2 {
		t.Fatalf("expected 2 fused results, got %d", len(rBias))
	}
	if rBias[0].ChunkID != "chk-d1" {
		t.Errorf("expected dense-biased winner chk-d1, got %s", rBias[0].ChunkID)
	}
	// Score check: dense rank1 = 1/61; sparse rank1 = 0.25/61.
	wantD := float64(1.0 / 61.0)
	wantS := float64(0.25 / 61.0)
	if abs(float64(rBias[0].Score)-wantD) > 1e-9 || abs(float64(rBias[1].Score)-wantS) > 1e-9 {
		t.Errorf("expected scores %v/%v, got %v/%v", wantD, wantS, rBias[0].Score, rBias[1].Score)
	}
}

func TestFusionPolicy_SparseCap(t *testing.T) {
	ctx := context.Background()

	dense := policyRetriever{results: []seam.SearchResult{result("chk-d1")}}
	sparse := policyRetriever{results: []seam.SearchResult{
		result("chk-s1"), result("chk-s2"), result("chk-s3"), result("chk-s4"), result("chk-s5"),
	}}

	h := hybrid.NewHybridRetriever(dense, sparse,
		hybrid.WithFusionPolicy(hybrid.FusionPolicy{
			DenseWeight:  1.0,
			SparseWeight: 1.0,
			SparseCap:    3,
			RRFK:         60,
		}))
	results, err := h.Retrieve(ctx, seam.RetrievalQuery{QueryText: "q", TopK: 10, Mode: seam.RetrievalHybrid})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only the top-3 sparse candidates may enter fusion.
	seen := map[string]bool{}
	for _, r := range results {
		seen[r.ChunkID] = true
	}
	if len(results) != 4 { // 1 dense + 3 sparse
		t.Fatalf("expected 4 fused results (cap 3), got %d: %+v", len(results), results)
	}
	for _, s := range []string{"chk-s1", "chk-s2", "chk-s3"} {
		if !seen[s] {
			t.Errorf("expected capped sparse candidate %s in results", s)
		}
	}
	if seen["chk-s4"] || seen["chk-s5"] {
		t.Errorf("sparse candidates beyond cap must be excluded: %+v", results)
	}
}

func TestFusionPolicy_DefaultReproducesBalanced(t *testing.T) {
	ctx := context.Background()

	dense := policyRetriever{results: []seam.SearchResult{result("chk-d1")}}
	sparse := policyRetriever{results: []seam.SearchResult{result("chk-s1")}}

	plain := hybrid.NewHybridRetriever(dense, sparse)
	withDefault := hybrid.NewHybridRetriever(dense, sparse, hybrid.WithFusionPolicy(hybrid.DefaultFusionPolicy()))

	r1, err := plain.Retrieve(ctx, seam.RetrievalQuery{QueryText: "q", TopK: 10, Mode: seam.RetrievalHybrid})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r2, err := withDefault.Retrieve(ctx, seam.RetrievalQuery{QueryText: "q", TopK: 10, Mode: seam.RetrievalHybrid})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r1) != len(r2) {
		t.Fatalf("default policy must reproduce balanced behavior: %d vs %d results", len(r1), len(r2))
	}
	for i := range r1 {
		if r1[i].ChunkID != r2[i].ChunkID || r1[i].Score != r2[i].Score {
			t.Errorf("default policy diverged from balanced at %d: %+v vs %+v", i, r1[i], r2[i])
		}
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
