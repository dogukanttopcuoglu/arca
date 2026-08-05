package store

import (
	"context"

	graphmodel "arca/internal/graph/model"
)

// GraphStore defines the seam for managing graph persistence and traversal
// operations. M7 (ADR-0038) uses entity-only node persistence; edge methods
// remain in the seam for future iterations but are not supported by the v1
// Qdrant adapter.
type GraphStore interface {
	// AddNode inserts or updates a Node vertex. For entity nodes carrying a
	// "chunk_ids" evidence property, the upsert is idempotent: existing chunk
	// IDs are unioned with the new ones.
	AddNode(ctx context.Context, node graphmodel.Node) error

	// AddEdge inserts a directed Edge connection.
	AddEdge(ctx context.Context, edge graphmodel.Edge) error

	// GetNode retrieves a Node by ID.
	GetNode(ctx context.Context, id string) (*graphmodel.Node, error)

	// FindNodeByName retrieves a Node by its normalized name property
	// (exact match on the lowercase "name" property).
	FindNodeByName(ctx context.Context, name string) (*graphmodel.Node, error)

	// ListEntityNodes returns every entity node in the store. The order is
	// unspecified; callers must apply their own deterministic ordering.
	ListEntityNodes(ctx context.Context) ([]graphmodel.Node, error)

	// DeleteByDocument removes all chunk evidence belonging to the given
	// document from every node; nodes left without evidence are deleted.
	DeleteByDocument(ctx context.Context, documentID string) error

	// Traverse performs breadth-first graph traversal up to maxDepth hops.
	Traverse(ctx context.Context, startNodeID string, maxDepth int) ([]graphmodel.Node, error)
}
