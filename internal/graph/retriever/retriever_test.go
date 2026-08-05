package retriever_test

import (
	"context"
	"testing"

	graphmodel "arca/internal/graph/model"
	graphretriever "arca/internal/graph/retriever"
	graphstore "arca/internal/graph/store"
	"arca/internal/indexing/store"
	"arca/internal/retrieval/seam"
)

// entityNode builds an entity node with the ADR-0038 payload shape.
func entityNode(id, name string, score float64, chunkIDs ...string) graphmodel.Node {
	return graphmodel.Node{
		ID:   id,
		Type: graphmodel.NodeTypeEntity,
		Properties: map[string]any{
			"name":           name,
			"canonical_name": name,
			"entity_type":    "organization",
			"score":          score,
			"chunk_ids":      chunkIDs,
		},
	}
}

func seededGraph(t *testing.T, content map[string]string) (*graphstore.InMemoryGraphStore, *store.InMemoryContentStore) {
	t.Helper()
	ctx := context.Background()
	gs := graphstore.NewInMemoryGraphStore()
	cs := store.NewInMemoryContentStore()

	nodes := []graphmodel.Node{
		entityNode("organization:world bank", "world bank", 1.0, "doc-a/notes/001", "doc-a/notes/005"),
		entityNode("organization:oxford", "oxford university", 0.9, "doc-b/body/002"),
		entityNode("person:rick rubin", "rick rubin", 0.9, "doc-c/body/001"),
	}
	for _, n := range nodes {
		if err := gs.AddNode(ctx, n); err != nil {
			t.Fatalf("seed node: %v", err)
		}
	}
	var contents []store.ChunkContent
	for id, text := range content {
		contents = append(contents, store.ChunkContent{ChunkID: id, ContentMarkdown: text})
	}
	if err := cs.PutContent(ctx, contents); err != nil {
		t.Fatalf("seed content: %v", err)
	}
	return gs, cs
}

func retrieve(t *testing.T, r seam.Retriever, query string, topK int, minScore float32) []seam.SearchResult {
	t.Helper()
	results, err := r.Retrieve(context.Background(), seam.RetrievalQuery{
		QueryText: query, TopK: topK, MinScore: minScore,
	})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	return results
}

func ids(results []seam.SearchResult) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.ChunkID
	}
	return out
}

func TestGraphRetriever_Scoring(t *testing.T) {
	gs, cs := seededGraph(t, map[string]string{
		"doc-a/notes/001": "The World Bank lends to developing countries.",
		"doc-a/notes/005": "World Bank annual report.",
		"doc-b/body/002":  "Oxford University is in England.",
		"doc-c/body/001":  "Rick Rubin produced Def Jam records.",
	})
	r := graphretriever.NewGraphRetriever(gs, cs)

	t.Run("full entity match scores coverage 1.0 and resolves content", func(t *testing.T) {
		results := retrieve(t, r, "What does the book say about World Bank?", 5, 0)
		if len(results) == 0 {
			t.Fatal("expected results for full entity match")
		}
		if results[0].ChunkID != "doc-a/notes/001" && results[0].ChunkID != "doc-a/notes/005" {
			t.Errorf("expected world bank chunks first, got %v", ids(results))
		}
		if results[0].ContentMarkdown == "" {
			t.Error("expected content resolved from ContentStore")
		}
	})

	t.Run("partial token match is damped by coverage", func(t *testing.T) {
		results := retrieve(t, r, "What is a bank?", 5, 0)
		// "bank" matches world bank with coverage 1/2 -> score 0.5 on both
		// evidenced chunks; other entities do not match.
		if len(results) != 2 {
			t.Fatalf("expected both world bank chunks, got %v", ids(results))
		}
		for _, res := range results {
			if res.Score > 0.51 || res.Score <= 0 {
				t.Errorf("expected damped score ~0.5, got %v", res.Score)
			}
		}
	})

	t.Run("repeated query tokens do not inflate coverage", func(t *testing.T) {
		results := retrieve(t, r, "What is a bank bank?", 5, 0)
		if len(results) != 2 {
			t.Fatalf("expected both world bank chunks, got %v", ids(results))
		}
		for _, res := range results {
			if res.Score > 0.51 {
				t.Errorf("expected coverage capped at 1/2 despite repeated token, got %v", res.Score)
			}
		}
	})

	t.Run("possessive query forms still match", func(t *testing.T) {
		results := retrieve(t, r, "What does the book say about World Bank's lending?", 5, 0)
		if len(results) != 2 {
			t.Fatalf("expected both world bank chunks despite possessive form, got %v", ids(results))
		}
	})

	t.Run("MinScore filters damped partial matches", func(t *testing.T) {
		results := retrieve(t, r, "What is a bank?", 5, 0.6)
		if len(results) != 0 {
			t.Errorf("expected no results below MinScore 0.6, got %v", ids(results))
		}
	})

	t.Run("full matches survive MinScore 0.6", func(t *testing.T) {
		results := retrieve(t, r, "What does the book say about World Bank?", 5, 0.6)
		if len(results) != 2 {
			t.Errorf("expected 2 full-match chunks, got %v", ids(results))
		}
	})
}

func TestGraphRetriever_Determinism(t *testing.T) {
	gs, cs := seededGraph(t, map[string]string{
		"doc-a/notes/001": "World Bank content.",
		"doc-a/notes/005": "More World Bank content.",
	})
	r := graphretriever.NewGraphRetriever(gs, cs)

	t.Run("consecutive calls produce identical lists", func(t *testing.T) {
		first := retrieve(t, r, "What does the book say about World Bank?", 5, 0)
		for i := 0; i < 3; i++ {
			again := retrieve(t, r, "What does the book say about World Bank?", 5, 0)
			if len(again) != len(first) {
				t.Fatalf("length drift: %d vs %d", len(again), len(first))
			}
			for j := range first {
				if again[j].ChunkID != first[j].ChunkID || again[j].Score != first[j].Score ||
					again[j].ContentMarkdown != first[j].ContentMarkdown {
					t.Fatalf("order/score/content drift at %d: %+v vs %+v", j, again[j], first[j])
				}
			}
		}
	})

	t.Run("equal scores keep stable chunk-ID ordering", func(t *testing.T) {
		results := retrieve(t, r, "What does the book say about World Bank?", 5, 0)
		if len(results) != 2 {
			t.Fatalf("expected 2 chunks, got %v", ids(results))
		}
		// The fixture yields two full matches with identical scores, so the
		// tie-break is always exercised; a fixture change that breaks the tie
		// must fail this test instead of silently skipping it.
		if results[0].Score != results[1].Score {
			t.Fatalf("fixture must produce equal scores, got %v vs %v", results[0].Score, results[1].Score)
		}
		if !(results[0].ChunkID < results[1].ChunkID) {
			t.Errorf("expected chunk-ID ascending tie-break, got %v", ids(results))
		}
	})
}

func TestGraphRetriever_TopKAndNoMatch(t *testing.T) {
	gs, cs := seededGraph(t, map[string]string{})
	r := graphretriever.NewGraphRetriever(gs, cs)

	t.Run("truncates to TopK", func(t *testing.T) {
		results := retrieve(t, r, "What does the book say about World Bank?", 1, 0)
		if len(results) != 1 {
			t.Errorf("expected TopK 1, got %v", ids(results))
		}
	})

	t.Run("no entity match returns empty", func(t *testing.T) {
		results := retrieve(t, r, "What is the capital of Atlantis?", 5, 0)
		if len(results) != 0 {
			t.Errorf("expected no results, got %v", ids(results))
		}
	})

	t.Run("empty query is rejected", func(t *testing.T) {
		_, err := r.Retrieve(context.Background(), seam.RetrievalQuery{QueryText: ""})
		if err == nil {
			t.Error("expected error for empty query")
		}
	})
}
