package store

import (
	"context"
	"fmt"
	"sync"

	graphmodel "arca/internal/graph/model"
)

// InMemoryGraphStore is a thread-safe in-memory graph database adapter.
type InMemoryGraphStore struct {
	mu    sync.RWMutex
	nodes map[string]graphmodel.Node
	edges map[string][]graphmodel.Edge
}

// NewInMemoryGraphStore constructs an InMemoryGraphStore instance.
func NewInMemoryGraphStore() *InMemoryGraphStore {
	return &InMemoryGraphStore{
		nodes: make(map[string]graphmodel.Node),
		edges: make(map[string][]graphmodel.Edge),
	}
}

// AddNode inserts or updates a Node.
func (s *InMemoryGraphStore) AddNode(ctx context.Context, node graphmodel.Node) error {
	if node.ID == "" {
		return fmt.Errorf("node ID cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.nodes[node.ID] = node
	return nil
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
