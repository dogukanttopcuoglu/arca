package store

import (
	"context"
	"fmt"
	"strings"
	"sync"

	graphmodel "arca/internal/graph/model"
)

// InMemoryGraphStore is a thread-safe in-memory graph database adapter.
type InMemoryGraphStore struct {
	mu     sync.RWMutex
	nodes  map[string]graphmodel.Node
	byName map[string]string // normalized name -> node ID
	edges  map[string][]graphmodel.Edge
}

// NewInMemoryGraphStore constructs an InMemoryGraphStore instance.
func NewInMemoryGraphStore() *InMemoryGraphStore {
	return &InMemoryGraphStore{
		nodes:  make(map[string]graphmodel.Node),
		byName: make(map[string]string),
		edges:  make(map[string][]graphmodel.Edge),
	}
}

// AddNode inserts or updates a Node. Entity nodes with a "chunk_ids"
// evidence property are upserted idempotently: existing chunk IDs are
// unioned with the incoming ones (ADR-0038). The "name" property is stored
// normalized (lowercased, trimmed) so FindNodeByName is case-insensitive
// across both adapters.
func (s *InMemoryGraphStore) AddNode(ctx context.Context, node graphmodel.Node) error {
	if node.ID == "" {
		return fmt.Errorf("node ID cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.nodes[node.ID]; ok {
		if node.Properties == nil {
			node.Properties = map[string]any{}
		}
		node.Properties["chunk_ids"] = unionStrings(chunkIDsFrom(existing), chunkIDsFrom(node))
	}
	if node.Properties == nil {
		node.Properties = map[string]any{}
	}
	if name, ok := node.Properties["name"].(string); ok {
		norm := strings.ToLower(strings.TrimSpace(name))
		node.Properties["name"] = norm
		s.byName[norm] = node.ID
	}
	s.nodes[node.ID] = node
	return nil
}

// FindNodeByName retrieves a Node by its normalized name: the input is
// lowercased and trimmed, then matched exactly against the node "name"
// property (ADR-0039 lexical normalization lives at the seam).
func (s *InMemoryGraphStore) FindNodeByName(ctx context.Context, name string) (*graphmodel.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := strings.ToLower(strings.TrimSpace(name))
	id, ok := s.byName[key]
	if !ok {
		return nil, fmt.Errorf("node not found by name: %s", name)
	}
	node, exists := s.nodes[id]
	if !exists {
		return nil, fmt.Errorf("node not found: %s", id)
	}
	return &node, nil
}

// DeleteByDocument removes every chunk evidence reference belonging to the
// document from all nodes; nodes left without evidence are deleted.
func (s *InMemoryGraphStore) DeleteByDocument(ctx context.Context, documentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, node := range s.nodes {
		ids := chunkIDsFrom(node)
		if ids == nil {
			continue
		}
		kept := ids[:0]
		for _, cid := range ids {
			if chunkBelongsToDocument(cid, documentID) {
				continue
			}
			kept = append(kept, cid)
		}
		if len(kept) == 0 {
			delete(s.nodes, id)
			if name, ok := node.Properties["name"].(string); ok {
				delete(s.byName, name)
			}
			continue
		}
		node.Properties["chunk_ids"] = kept
		s.nodes[id] = node
	}
	return nil
}

// unionStrings appends strings from b that are not already in a, preserving
// order and uniqueness.
func unionStrings(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range a {
		seen[s] = true
		out = append(out, s)
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// chunkBelongsToDocument reports whether a chunk ID belongs to the document:
// chunk IDs are formatted "documentID/<section>/<ordinal>".
func chunkBelongsToDocument(chunkID, documentID string) bool {
	prefix := documentID + "/"
	return len(chunkID) >= len(prefix) && chunkID[:len(prefix)] == prefix
}

// AddEdge inserts a directed Edge.
func (s *InMemoryGraphStore) AddEdge(ctx context.Context, edge graphmodel.Edge) error {
	if edge.From == "" || edge.To == "" {
		return fmt.Errorf("edge From and To IDs cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.edges[edge.From] = append(s.edges[edge.From], edge)
	return nil
}

// GetNode retrieves a Node by ID.
func (s *InMemoryGraphStore) GetNode(ctx context.Context, id string) (*graphmodel.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	node, exists := s.nodes[id]
	if !exists {
		return nil, fmt.Errorf("node not found: %s", id)
	}
	return &node, nil
}

// Traverse performs BFS graph traversal up to maxDepth.
func (s *InMemoryGraphStore) Traverse(ctx context.Context, startNodeID string, maxDepth int) ([]graphmodel.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, exists := s.nodes[startNodeID]; !exists {
		return nil, fmt.Errorf("start node not found: %s", startNodeID)
	}

	visited := make(map[string]bool)
	visited[startNodeID] = true

	queue := []string{startNodeID}
	var result []graphmodel.Node

	depth := 0
	for len(queue) > 0 && depth < maxDepth {
		levelSize := len(queue)
		for i := 0; i < levelSize; i++ {
			curr := queue[0]
			queue = queue[1:]

			for _, edge := range s.edges[curr] {
				if !visited[edge.To] {
					visited[edge.To] = true
					if targetNode, ok := s.nodes[edge.To]; ok {
						result = append(result, targetNode)
						queue = append(queue, edge.To)
					}
				}
			}
		}
		depth++
	}

	return result, nil
}
