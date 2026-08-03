package store

import (
	"context"

	"arca/internal/indexing/model"
)

// VectorSearchQuery encapsulates parameters for nearest-neighbor vector similarity queries.
type VectorSearchQuery struct {
	Vector   []float32            `json:"vector"`
	TopK     int                  `json:"top_k"`
	Filter   model.MetadataFilter `json:"filter,omitempty"`
	MinScore float32              `json:"min_score,omitempty"`
}

// VectorSearchResult encapsulates a single nearest-neighbor match returned by VectorStore.
type VectorSearchResult struct {
	ID       string               `json:"id"`
	Score    float32              `json:"score"`
	Metadata model.VectorMetadata `json:"metadata"`
}

// VectorStore defines the low-level persistence abstraction for vector storage, search, and deletion.
type VectorStore interface {
	// UpsertPoints inserts or updates vector points in-place based on Point ID.
	UpsertPoints(ctx context.Context, points []VectorPoint) error

	// SearchVector executes nearest-neighbor similarity search using VectorSearchQuery.
	SearchVector(ctx context.Context, query VectorSearchQuery) ([]VectorSearchResult, error)

	// ListPoints enumerates all stored points matching the filter without ranking.
	// This is a read operation for enumeration (e.g. differential indexing), not a
	// similarity search, so it never truncates by TopK and never requires a query vector.
	ListPoints(ctx context.Context, filter model.MetadataFilter) ([]VectorPoint, error)

	// Delete removes matching vector points based on MetadataFilter.
	Delete(ctx context.Context, filter model.MetadataFilter) error

	// Health checks database connection health and availability.
	Health(ctx context.Context) error
}
