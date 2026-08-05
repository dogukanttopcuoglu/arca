package store_test

import (
	"context"
	"testing"

	graphmodel "arca/internal/graph/model"
	graphstore "arca/internal/graph/store"
)

func entityNode(id, name string, chunkIDs ...string) graphmodel.Node {
	return graphmodel.Node{
		ID:   id,
		Type: graphmodel.NodeTypeEntity,
		Properties: map[string]any{
			"name":           name,
			"canonical_name": name,
			"entity_type":    "organization",
			"score":          0.9,
			"chunk_ids":      chunkIDs,
		},
	}
}

func chunkIDs(n *graphmodel.Node) []string {
	ids, _ := n.Properties["chunk_ids"].([]string)
	return ids
}

func TestInMemoryGraphStore_NodeEvidence(t *testing.T) {
	ctx := context.Background()
	s := graphstore.NewInMemoryGraphStore()

	t.Run("AddNode unions chunk IDs idempotently", func(t *testing.T) {
		if err := s.AddNode(ctx, entityNode("organization:world bank", "world bank", "doc-a/notes/001")); err != nil {
			t.Fatalf("first add: %v", err)
		}
		if err := s.AddNode(ctx, entityNode("organization:world bank", "world bank", "doc-a/notes/001", "doc-a/notes/005")); err != nil {
			t.Fatalf("second add: %v", err)
		}
		node, err := s.GetNode(ctx, "organization:world bank")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		ids := chunkIDs(node)
		if len(ids) != 2 {
			t.Fatalf("expected union of 2 chunk ids, got %v", ids)
		}
	})

	t.Run("FindNodeByName matches the normalized name", func(t *testing.T) {
		node, err := s.FindNodeByName(ctx, "World Bank")
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if node.ID != "organization:world bank" {
			t.Errorf("expected node id, got %q", node.ID)
		}
	})

	t.Run("FindNodeByName is deterministic for absent nodes", func(t *testing.T) {
		if _, err := s.FindNodeByName(ctx, "def jam"); err == nil {
			t.Error("expected error for unknown name")
		}
	})

	t.Run("AddNode normalizes mixed-case names for the name index", func(t *testing.T) {
		if err := s.AddNode(ctx, entityNode("organization:mixed case", "Mixed Case Corp", "doc-x/body/001")); err != nil {
			t.Fatalf("add: %v", err)
		}
		node, err := s.FindNodeByName(ctx, "MIXED CASE CORP")
		if err != nil {
			t.Fatalf("find mixed-case: %v", err)
		}
		if name, _ := node.Properties["name"].(string); name != "mixed case corp" {
			t.Errorf("expected normalized stored name, got %q", name)
		}
	})

	t.Run("DeleteByDocument leaves no stale name index entries", func(t *testing.T) {
		if err := s.DeleteByDocument(ctx, "doc-x"); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := s.FindNodeByName(ctx, "Mixed Case Corp"); err == nil {
			t.Error("expected clean miss after node deletion (no stale index)")
		}
	})

	t.Run("DeleteByDocument removes only that document's chunk evidence", func(t *testing.T) {
		if err := s.AddNode(ctx, entityNode("organization:oxford", "oxford", "doc-a/notes/001", "doc-b/body/002")); err != nil {
			t.Fatalf("add: %v", err)
		}
		if err := s.DeleteByDocument(ctx, "doc-a"); err != nil {
			t.Fatalf("delete doc: %v", err)
		}
		node, err := s.GetNode(ctx, "organization:oxford")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		ids := chunkIDs(node)
		if len(ids) != 1 || ids[0] != "doc-b/body/002" {
			t.Fatalf("expected only doc-b evidence, got %v", ids)
		}
	})

	t.Run("DeleteByDocument removes nodes left without evidence", func(t *testing.T) {
		if err := s.DeleteByDocument(ctx, "doc-b"); err != nil {
			t.Fatalf("delete doc-b: %v", err)
		}
		if _, err := s.GetNode(ctx, "organization:oxford"); err == nil {
			t.Error("expected node removal after evidence loss")
		}
	})
}

func TestCalculateNodePointID(t *testing.T) {
	id1 := graphmodel.CalculatePointID("organization:world bank")
	id2 := graphmodel.CalculatePointID("organization:world bank")
	if id1 != id2 {
		t.Fatalf("expected deterministic point id, got %q vs %q", id1, id2)
	}
	if len(id1) != 36 {
		t.Fatalf("expected UUID-format point id, got %q", id1)
	}
	if id1 == graphmodel.CalculatePointID("organization:world-bank") {
		t.Error("expected distinct ids for distinct node ids")
	}
}
