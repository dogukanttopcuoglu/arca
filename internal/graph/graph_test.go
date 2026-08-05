package graph_test

import (
	"context"
	"testing"

	graphmodel "arca/internal/graph/model"
	graphretriever "arca/internal/graph/retriever"
	graphstore "arca/internal/graph/store"
	"arca/internal/indexing/store"
	retrievalseam "arca/internal/retrieval/seam"
)

func TestInMemoryGraphStore(t *testing.T) {
	ctx := context.Background()
	store := graphstore.NewInMemoryGraphStore()

	t.Run("adds nodes and edges, then performs graph traversal", func(t *testing.T) {
		node1 := graphmodel.Node{
			ID:   "node-doc-1",
			Type: graphmodel.NodeTypeDocument,
			Properties: map[string]any{
				"title": "The Creative Act",
			},
		}
		node2 := graphmodel.Node{
			ID:   "node-concept-1",
			Type: graphmodel.NodeTypeConcept,
			Properties: map[string]any{
				"name": "Flow State",
			},
		}

		if err := store.AddNode(ctx, node1); err != nil {
			t.Fatalf("unexpected error adding node1: %v", err)
		}
		if err := store.AddNode(ctx, node2); err != nil {
			t.Fatalf("unexpected error adding node2: %v", err)
		}

		edge := graphmodel.Edge{
			From:     "node-doc-1",
			To:       "node-concept-1",
			Relation: graphmodel.RelationContains,
		}
		if err := store.AddEdge(ctx, edge); err != nil {
			t.Fatalf("unexpected error adding edge: %v", err)
		}

		neighbors, err := store.Traverse(ctx, "node-doc-1", 1)
		if err != nil {
			t.Fatalf("unexpected error traversing graph: %v", err)
		}

		if len(neighbors) != 1 {
			t.Fatalf("expected 1 neighbor, got %d", len(neighbors))
		}
		if neighbors[0].ID != "node-concept-1" {
			t.Errorf("expected neighbor 'node-concept-1', got %s", neighbors[0].ID)
		}
	})
}

func TestGraphRetriever_ImplementsRetrieverSeam(t *testing.T) {
	ctx := context.Background()
	gs := graphstore.NewInMemoryGraphStore()
	cs := store.NewInMemoryContentStore()

	_ = gs.AddNode(ctx, graphmodel.Node{
		ID:   "organization:world bank",
		Type: graphmodel.NodeTypeEntity,
		Properties: map[string]any{
			"name":      "world bank",
			"score":     1.0,
			"chunk_ids": []string{"doc-1/notes/001"},
		},
	})
	_ = cs.PutContent(ctx, []store.ChunkContent{{
		ChunkID: "doc-1/notes/001", ContentMarkdown: "The World Bank lends to developing countries.",
	}})

	gr := graphretriever.NewGraphRetriever(gs, cs)

	t.Run("retrieves entity evidence adhering to Retriever seam", func(t *testing.T) {
		results, err := gr.Retrieve(ctx, retrievalseam.RetrievalQuery{
			QueryText: "What does the book say about World Bank?",
			TopK:      5,
		})

		if err != nil {
			t.Fatalf("unexpected error during GraphRetriever.Retrieve: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 entity-evidence result, got %d", len(results))
		}
		if results[0].ChunkID != "doc-1/notes/001" {
			t.Errorf("expected entity-evidence chunk, got %s", results[0].ChunkID)
		}
		if results[0].ContentMarkdown == "" {
			t.Error("expected content resolved from ContentStore")
		}
	})
}
