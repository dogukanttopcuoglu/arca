package graphfusion_test

import (
	"context"
	"testing"

	indexingmodel "arca/internal/indexing/model"
	"arca/internal/retrieval/graphfusion"
	"arca/internal/retrieval/seam"
)

// scriptedRetriever returns a fixed result list per call.
type scriptedRetriever struct {
	results []seam.SearchResult
	calls   int
}

func (s *scriptedRetriever) Retrieve(ctx context.Context, q seam.RetrievalQuery) ([]seam.SearchResult, error) {
	s.calls++
	return s.results, nil
}

func res(id string, score float32) seam.SearchResult {
	return seam.SearchResult{
		ChunkID: id,
		Score:   score,
		Metadata: indexingmodel.VectorMetadata{
			ChunkID: id,
		},
	}
}

func TestGraphFusionRetriever(t *testing.T) {
	ctx := context.Background()
	dense := &scriptedRetriever{results: []seam.SearchResult{
		res("d1", 0.9), res("d2", 0.8), res("d3", 0.7),
	}}
	graph := &scriptedRetriever{results: []seam.SearchResult{
		res("g1", 1.0), res("g2", 0.6), res("d2", 0.5),
	}}

	t.Run("GraphWeight 0 returns the dense stream unchanged", func(t *testing.T) {
		fr := graphfusion.NewGraphFusionRetriever(dense, graph, graphfusion.GraphFusionConfig{})
		results, err := fr.Retrieve(ctx, seam.RetrievalQuery{QueryText: "q", TopK: 5})
		if err != nil {
			t.Fatalf("retrieve: %v", err)
		}
		want := []string{"d1", "d2", "d3"}
		if len(results) != len(want) {
			t.Fatalf("expected dense-only results, got %v", ids(results))
		}
		for i := range want {
			if results[i].ChunkID != want[i] {
				t.Fatalf("expected dense order preserved, got %v", ids(results))
			}
		}
		if results[0].Score != 0.9 {
			t.Errorf("expected dense scores preserved, got %v", results[0].Score)
		}
	})

	t.Run("positive GraphWeight fuses dense and graph streams with RRF", func(t *testing.T) {
		fr := graphfusion.NewGraphFusionRetriever(dense, graph, graphfusion.GraphFusionConfig{
			DenseWeight: 1.0, GraphWeight: 1.0,
		})
		results, err := fr.Retrieve(ctx, seam.RetrievalQuery{QueryText: "q", TopK: 5})
		if err != nil {
			t.Fatalf("retrieve: %v", err)
		}
		// d2 appears in both streams -> highest RRF score, must rank first.
		if len(results) == 0 || results[0].ChunkID != "d2" {
			t.Fatalf("expected the cross-stream chunk first, got %v", ids(results))
		}
		if len(results) != 5 {
			t.Errorf("expected 5 fused results, got %d: %v", len(results), ids(results))
		}
		// All distinct chunks from both streams present.
		got := map[string]bool{}
		for _, r := range results {
			got[r.ChunkID] = true
		}
		for _, cid := range []string{"d1", "d2", "d3", "g1", "g2"} {
			if !got[cid] {
				t.Errorf("expected %s in fused results", cid)
			}
		}
	})

	t.Run("empty graph stream degrades to the dense stream", func(t *testing.T) {
		emptyGraph := &scriptedRetriever{}
		fr := graphfusion.NewGraphFusionRetriever(dense, emptyGraph, graphfusion.GraphFusionConfig{
			DenseWeight: 1.0, GraphWeight: 1.0,
		})
		results, err := fr.Retrieve(ctx, seam.RetrievalQuery{QueryText: "q", TopK: 5})
		if err != nil {
			t.Fatalf("retrieve: %v", err)
		}
		if len(results) != 3 || results[0].ChunkID != "d1" {
			t.Fatalf("expected dense-only degradation, got %v", ids(results))
		}
	})

	t.Run("SetConfig drives the sweep surface", func(t *testing.T) {
		fr := graphfusion.NewGraphFusionRetriever(dense, graph, graphfusion.GraphFusionConfig{})
		fr.SetConfig(graphfusion.GraphFusionConfig{DenseWeight: 1.0, GraphWeight: 1.0})
		results, err := fr.Retrieve(ctx, seam.RetrievalQuery{QueryText: "q", TopK: 5})
		if err != nil {
			t.Fatalf("retrieve: %v", err)
		}
		if results[0].ChunkID != "d2" {
			t.Errorf("expected fused ranking after SetConfig, got %v", ids(results))
		}
	})

	t.Run("fused ranking is deterministic across calls", func(t *testing.T) {
		fr := graphfusion.NewGraphFusionRetriever(dense, graph, graphfusion.GraphFusionConfig{
			DenseWeight: 1.0, GraphWeight: 1.0,
		})
		first, err := fr.Retrieve(ctx, seam.RetrievalQuery{QueryText: "q", TopK: 5})
		if err != nil {
			t.Fatalf("retrieve: %v", err)
		}
		for i := 0; i < 3; i++ {
			again, err := fr.Retrieve(ctx, seam.RetrievalQuery{QueryText: "q", TopK: 5})
			if err != nil {
				t.Fatalf("retrieve: %v", err)
			}
			if len(again) != len(first) {
				t.Fatalf("length drift: %d vs %d", len(again), len(first))
			}
			for j := range first {
				if again[j].ChunkID != first[j].ChunkID || again[j].Score != first[j].Score {
					t.Fatalf("fused order drift at %d: %+v vs %+v", j, again[j], first[j])
				}
			}
		}
	})

	t.Run("truncates to TopK", func(t *testing.T) {
		fr := graphfusion.NewGraphFusionRetriever(dense, graph, graphfusion.GraphFusionConfig{
			DenseWeight: 1.0, GraphWeight: 1.0,
		})
		results, err := fr.Retrieve(ctx, seam.RetrievalQuery{QueryText: "q", TopK: 2})
		if err != nil {
			t.Fatalf("retrieve: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected TopK 2, got %d", len(results))
		}
	})
}

func ids(results []seam.SearchResult) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.ChunkID
	}
	return out
}
