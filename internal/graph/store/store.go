package store

import (
	"context"

	graphmodel "arca/internal/graph/model"
)

// GraphStore defines the seam for managing graph persistence and traversal operations.
type GraphStore interface {
	// AddNode inserts or updates a Node vertex.
	AddNode(ctx context.Context, node graphmodel.Node) error

	// AddEdge inserts a directed Edge connection.
	AddEdge(ctx context.Context, edge graphmodel.Edge) error

	// GetNode retrieves a Node by ID.
	GetNode(ctx context.Context, id string) (*graphmodel.Node, error)

	// Traverse performs breadth-first graph traversal up to maxDepth hops.
	Traverse(ctx context.Context, startNodeID string, maxDepth int) ([]graphmodel.Node, error)
}
