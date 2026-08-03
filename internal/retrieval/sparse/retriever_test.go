package sparse_test

import (
	"context"
	"testing"

	indexingmodel "arca/internal/indexing/model"
	"arca/internal/indexing/sparse"
	"arca/internal/indexing/store"
	"arca/internal/retrieval/seam"
	retrievalsparse "arca/internal/retrieval/sparse"
)

// fakeQueryEncoder encodes query text into a fixed sparse vector so tests
// control exactly what the store searches with.
type fakeQueryEncoder struct {
	vec sparse.SparseVector
}

func (e fakeQueryEncoder) Encode(ctx context.Context, text string) (sparse.SparseVector, error) {
	return e.vec, nil
}

func TestSparseRetriever(t *testing.T) {
	ctx := context.Background()

	storeImpl := store.NewInMemoryVectorStore()
	contentStore := store.NewInMemoryContentStore()

	// Query vector: index 1 = 1.0. Points: chk-1 weight 2.0 at idx 1 (dot 2.0),
	// chk-2 weight 4.0 at idx 1 (dot 4.0), chk-3 weight 9.0 at idx 2 (dot 0).
	pts := []store.VectorPoint{
		{
			ID:     "pt-1",
			Vector: []float32{1, 0, 0},
			Sparse: &sparse.SparseVector{Indices: []uint32{1}, Values: []float32{2.0}},
			Metadata: indexingmodel.VectorMetadata{
				DocumentID: "doc-1", ChunkID: "chk-1", SectionPath: "A", ContentHash: "h1",
			},
		},
		{
			ID:     "pt-2",
			Vector: []float32{0, 1, 0},
			Sparse: &sparse.SparseVector{Indices: []uint32{1}, Values: []float32{4.0}},
			Metadata: indexingmodel.VectorMetadata{
				DocumentID: "doc-1", ChunkID: "chk-2", SectionPath: "B", ContentHash: "h2",
			},
		},
		{
			ID:     "pt-3",
			Vector: []float32{0, 0, 1},
			Sparse: &sparse.SparseVector{Indices: []uint32{2}, Values: []float32{9.0}},
			Metadata: indexingmodel.VectorMetadata{
				DocumentID: "doc-1", ChunkID: "chk-3", SectionPath: "C", ContentHash: "h3",
			},
		},
	}
	if err := storeImpl.UpsertPoints(ctx, pts); err != nil {
		t.Fatalf("failed to seed store: %v", err)
	}
	if err := contentStore.PutContent(ctx, []store.ChunkContent{
		{ChunkID: "chk-1", ContentMarkdown: "content one"},
		{ChunkID: "chk-2", ContentMarkdown: "content two"},
		{ChunkID: "chk-3", ContentMarkdown: "content three"},
	}); err != nil {
		t.Fatalf("failed to seed content: %v", err)
	}

	encoder := fakeQueryEncoder{vec: sparse.SparseVector{Indices: []uint32{1}, Values: []float32{1.0}}}
	retriever := retrievalsparse.NewSparseRetriever(encoder, storeImpl, contentStore)

	t.Run("encodes the query and ranks by sparse dot product", func(t *testing.T) {
		results, err := retriever.Retrieve(ctx, seam.RetrievalQuery{
			QueryText: "creativity",
			TopK:      5,
			Mode:      seam.RetrievalSparse,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("expected 3 matches (zero-dot match included at MinScore 0), got %d", len(results))
		}
		if results[0].ChunkID != "chk-2" || results[1].ChunkID != "chk-1" {
			t.Errorf("expected chk-2 then chk-1 by score, got %s %s", results[0].ChunkID, results[1].ChunkID)
		}
		if results[0].Score != 4.0 || results[1].Score != 2.0 {
			t.Errorf("expected scores 4.0, 2.0, got %v %v", results[0].Score, results[1].Score)
		}
		if results[0].ContentMarkdown != "content two" {
			t.Errorf("expected content resolved, got %q", results[0].ContentMarkdown)
		}
	})

	t.Run("applies TopK and MinScore like the dense retriever", func(t *testing.T) {
		results, err := retriever.Retrieve(ctx, seam.RetrievalQuery{
			QueryText: "creativity",
			TopK:      1,
			MinScore:  3.0,
			Mode:      seam.RetrievalSparse,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 || results[0].ChunkID != "chk-2" {
			t.Errorf("expected only chk-2 above threshold 3.0, got %+v", results)
		}
	})

	t.Run("populates retrieval statistics when requested", func(t *testing.T) {
		stats := &seam.RetrievalStats{}
		_, err := retriever.Retrieve(ctx, seam.RetrievalQuery{
			QueryText: "creativity",
			TopK:      5,
			Mode:      seam.RetrievalSparse,
			Stats:     stats,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stats.Candidates != 3 || stats.TopKRequested != 5 || stats.TopKReturned != 3 || stats.MinScore != 0 {
			t.Errorf("unexpected stats: %+v", stats)
		}
	})

	t.Run("returns deterministic ordering on tied scores", func(t *testing.T) {
		// Two points with identical weight 5.0 at index 1.
		tieStore := store.NewInMemoryVectorStore()
		for _, c := range []struct{ id, chunk string }{
			{"pt-b", "chk-b"}, {"pt-a", "chk-a"},
		} {
			_ = tieStore.UpsertPoints(ctx, []store.VectorPoint{{
				ID:     c.id,
				Vector: []float32{1, 0, 0},
				Sparse: &sparse.SparseVector{Indices: []uint32{1}, Values: []float32{5.0}},
				Metadata: indexingmodel.VectorMetadata{
					DocumentID: "doc-1", ChunkID: c.chunk, ContentHash: "h-" + c.chunk,
				},
			}})
		}
		tie := retrievalsparse.NewSparseRetriever(encoder, tieStore, store.NewInMemoryContentStore())
		r1, err := tie.Retrieve(ctx, seam.RetrievalQuery{QueryText: "q", TopK: 5, Mode: seam.RetrievalSparse})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r2, err := tie.Retrieve(ctx, seam.RetrievalQuery{QueryText: "q", TopK: 5, Mode: seam.RetrievalSparse})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(r1) != 2 || len(r2) != 2 {
			t.Fatalf("expected 2 results each, got %d and %d", len(r1), len(r2))
		}
		if r1[0].ChunkID != r2[0].ChunkID || r1[1].ChunkID != r2[1].ChunkID {
			t.Errorf("tied ordering must be deterministic: %s,%s vs %s,%s",
				r1[0].ChunkID, r1[1].ChunkID, r2[0].ChunkID, r2[1].ChunkID)
		}
		// Tie-break rule: ascending ChunkID.
		if r1[0].ChunkID != "chk-a" {
			t.Errorf("expected ChunkID ascending tie-break, got %s first", r1[0].ChunkID)
		}
	})
}
