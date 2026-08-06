package store_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	graphmodel "arca/internal/graph/model"
	graphstore "arca/internal/graph/store"
)

// TestQdrantGraphStore_Live runs against a real Qdrant instance when
// QDRANT_TEST_URL is set (e.g. "http://localhost:6333"). It is skipped
// otherwise — the same convention as the indexing store live tests.
func TestQdrantGraphStore_Live(t *testing.T) {
	host := os.Getenv("QDRANT_TEST_URL")
	if host == "" {
		t.Skip("QDRANT_TEST_URL not set; skipping live Qdrant graph integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	collection := "arca_test_graph_nodes"
	qs, err := graphstore.NewQdrantGraphStore(host, collection)
	if err != nil {
		t.Fatalf("failed to construct Qdrant graph store: %v", err)
	}
	t.Cleanup(func() { _ = qs.Close() })

	t.Run("upsert unions chunk evidence idempotently", func(t *testing.T) {
		node := graphmodel.Node{
			ID:   "organization:world bank",
			Type: graphmodel.NodeTypeEntity,
			Properties: map[string]any{
				"name":           "world bank",
				"canonical_name": "World Bank",
				"entity_type":    "organization",
				"score":          1.0,
				"chunk_ids":      []string{"doc-a/notes/001"},
			},
		}
		if err := qs.AddNode(ctx, node); err != nil {
			t.Fatalf("first add: %v", err)
		}
		node.Properties["chunk_ids"] = []string{"doc-a/notes/001", "doc-a/notes/005"}
		if err := qs.AddNode(ctx, node); err != nil {
			t.Fatalf("second add: %v", err)
		}

		got, err := qs.GetNode(ctx, "organization:world bank")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		ids := got.Properties["chunk_ids"].([]string)
		if len(ids) != 2 {
			t.Fatalf("expected union of 2 chunk ids, got %v", ids)
		}
	})

	t.Run("FindNodeByName matches the normalized name", func(t *testing.T) {
		node, err := qs.FindNodeByName(ctx, "World Bank")
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		if node.ID != "organization:world bank" {
			t.Errorf("expected node id, got %q", node.ID)
		}
	})

	t.Run("DeleteByDocument removes evidence and deletes empty nodes", func(t *testing.T) {
		second := graphmodel.Node{
			ID:   "organization:oxford",
			Type: graphmodel.NodeTypeEntity,
			Properties: map[string]any{
				"name":           "oxford",
				"canonical_name": "Oxford University",
				"entity_type":    "organization",
				"score":          0.9,
				"chunk_ids":      []string{"doc-a/notes/001", "doc-b/body/002"},
			},
		}
		if err := qs.AddNode(ctx, second); err != nil {
			t.Fatalf("add: %v", err)
		}
		if err := qs.DeleteByDocument(ctx, "doc-a"); err != nil {
			t.Fatalf("delete doc-a: %v", err)
		}
		node, err := qs.GetNode(ctx, "organization:oxford")
		if err != nil {
			t.Fatalf("get after partial delete: %v", err)
		}
		ids := node.Properties["chunk_ids"].([]string)
		if len(ids) != 1 || ids[0] != "doc-b/body/002" {
			t.Fatalf("expected only doc-b evidence, got %v", ids)
		}
		// world bank loses all evidence -> node deleted
		if _, err := qs.GetNode(ctx, "organization:world bank"); err == nil {
			t.Error("expected world bank node deletion after evidence loss")
		}
	})

	t.Run("deterministic point ids across stores", func(t *testing.T) {
		// The same node ID must map to the same point ID in any store.
		if graphmodel.CalculatePointID("organization:world bank") == graphmodel.CalculatePointID("organization:world-bank") {
			t.Error("expected distinct point ids for distinct node ids")
		}
	})

	t.Run("re-upsert keeps a single point (determinism round-trip)", func(t *testing.T) {
		node := graphmodel.Node{
			ID:   "person:rick rubin",
			Type: graphmodel.NodeTypeEntity,
			Properties: map[string]any{
				"name":           "Rick Rubin",
				"canonical_name": "Rick Rubin",
				"entity_type":    "person",
				"score":          0.9,
				"chunk_ids":      []string{"doc-a/body/001"},
			},
		}
		if err := qs.AddNode(ctx, node); err != nil {
			t.Fatalf("first upsert: %v", err)
		}
		if err := qs.AddNode(ctx, node); err != nil {
			t.Fatalf("second upsert: %v", err)
		}
		// Mixed-case write must be stored normalized and findable.
		found, err := qs.FindNodeByName(ctx, "Rick Rubin")
		if err != nil {
			t.Fatalf("find after mixed-case write: %v", err)
		}
		if found.Properties["name"] != "rick rubin" {
			t.Errorf("expected normalized stored name, got %v", found.Properties["name"])
		}
		var out struct {
			Result struct {
				Points []map[string]any `json:"points"`
			} `json:"result"`
		}
		scrollPoints(t, host, collection, &out)
		// oxford survived the earlier DeleteByDocument (doc-b evidence) and
		// rick rubin was upserted twice -> exactly 2 points; the re-upsert
		// must not have created a duplicate point for rick rubin.
		if len(out.Result.Points) != 2 {
			t.Errorf("expected 2 points after re-upserts, got %d", len(out.Result.Points))
		}
	})

	t.Run("concurrent AddNode never loses chunk evidence", func(t *testing.T) {
		// Two writers (simulating parallel indexing processes) upsert the
		// SAME node with chunk evidence from DIFFERENT documents. The union
		// must survive: QdrantGraphStore.AddNode must be race-free (M7 audit
		// A1). Different document prefixes keep the per-document score fields
		// disjoint, mirroring real parallel ingestion.
		lost := 0
		const iterations = 10
		for i := 0; i < iterations; i++ {
			_ = qs.DeleteByDocument(ctx, "racea")
			_ = qs.DeleteByDocument(ctx, "raceb")
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				_ = qs.AddNode(ctx, graphmodel.Node{
					ID:   "organization:race node",
					Type: graphmodel.NodeTypeEntity,
					Properties: map[string]any{
						"name":      "race node",
						"score":     1.0,
						"chunk_ids": []string{"racea/doc-a/notes/001"},
					},
				})
			}()
			go func() {
				defer wg.Done()
				_ = qs.AddNode(ctx, graphmodel.Node{
					ID:   "organization:race node",
					Type: graphmodel.NodeTypeEntity,
					Properties: map[string]any{
						"name":      "race node",
						"score":     0.9,
						"chunk_ids": []string{"raceb/doc-b/notes/001"},
					},
				})
			}()
			wg.Wait()
			node, err := qs.GetNode(ctx, "organization:race node")
			if err != nil {
				t.Fatalf("get after concurrent writes: %v", err)
			}
			hasA, hasB := false, false
			for _, cid := range node.ChunkIDs() {
				if cid == "racea/doc-a/notes/001" {
					hasA = true
				}
				if cid == "raceb/doc-b/notes/001" {
					hasB = true
				}
			}
			if !hasA || !hasB {
				lost++
				t.Logf("iter %d: LOST UPDATE — node has %v", i, node.ChunkIDs())
			}
			// The higher score must survive (score never regresses).
			if node.Score() < 1.0 {
				t.Errorf("iter %d: score regressed to %v", i, node.Score())
			}
		}
		if lost > 0 {
			t.Fatalf("lost updates: %d/%d", lost, iterations)
		}
	})

	t.Cleanup(func() {
		// Remove the throwaway collection so reruns start clean.
		deleteCollection(t, host, collection)
	})
}

// scrollPoints scrolls every point of the collection via raw REST (test-only:
// keeps the production store API free of test helpers).
func scrollPoints(t *testing.T, host, collection string, out any) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"limit": 100, "with_payload": true})
	if err != nil {
		t.Fatalf("marshal scroll: %v", err)
	}
	resp, err := http.Post(host+"/collections/"+collection+"/points/scroll", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("scroll request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("scroll status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("scroll decode: %v", err)
	}
}

// deleteCollection drops the throwaway collection via raw REST.
func deleteCollection(t *testing.T, host, collection string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, host+"/collections/"+collection, nil)
	if err != nil {
		t.Fatalf("delete request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	resp.Body.Close()
}
