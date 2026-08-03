package seam_test

import (
	"testing"

	indexingmodel "arca/internal/indexing/model"
	"arca/internal/retrieval/seam"
)

func sres(id string) seam.SearchResult {
	return seam.SearchResult{ChunkID: id, Metadata: indexingmodel.VectorMetadata{ChunkID: id}}
}

func TestMergeRankedLists(t *testing.T) {
	t.Run("interleaves ranked lists by rank", func(t *testing.T) {
		l1 := []seam.SearchResult{sres("a"), sres("b"), sres("c")}
		l2 := []seam.SearchResult{sres("x"), sres("y")}

		merged := seam.MergeRankedLists([][]seam.SearchResult{l1, l2}, 10)
		got := ids(merged)
		want := []string{"a", "x", "b", "y", "c"}
		if len(got) != len(want) {
			t.Fatalf("expected %v, got %v", want, got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("expected %v, got %v", want, got)
			}
		}
	})

	t.Run("deduplicates by chunk id keeping the first occurrence", func(t *testing.T) {
		l1 := []seam.SearchResult{sres("a"), sres("b")}
		l2 := []seam.SearchResult{sres("b"), sres("c")}

		merged := seam.MergeRankedLists([][]seam.SearchResult{l1, l2}, 10)
		got := ids(merged)
		want := []string{"a", "b", "c"}
		if len(got) != len(want) {
			t.Fatalf("expected %v, got %v", want, got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("expected %v, got %v", want, got)
			}
		}
	})

	t.Run("truncates to the top-k contract", func(t *testing.T) {
		l1 := []seam.SearchResult{sres("a"), sres("b"), sres("c")}
		l2 := []seam.SearchResult{sres("x"), sres("y"), sres("z")}

		merged := seam.MergeRankedLists([][]seam.SearchResult{l1, l2}, 3)
		if len(merged) != 3 {
			t.Fatalf("expected 3 results, got %d: %v", len(merged), ids(merged))
		}
	})

	t.Run("returns empty for empty inputs", func(t *testing.T) {
		if got := seam.MergeRankedLists(nil, 5); len(got) != 0 {
			t.Errorf("expected empty result, got %v", got)
		}
	})
}

func ids(rs []seam.SearchResult) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.ChunkID
	}
	return out
}
