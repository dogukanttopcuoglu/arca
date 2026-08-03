package store

import (
	"context"

	"arca/internal/indexing/model"
	"arca/internal/indexing/sparse"
)

// VectorSearchQuery encapsulates parameters for nearest-neighbor vector similarity queries.
// Exactly one of Vector (dense) or Sparse must be set; the store executes whichever
// representation is present — it has no retrieval-mode logic.
type VectorSearchQuery struct {
	Vector   []float32            `json:"vector"`
	Sparse   *sparse.SparseVector `json:"sparse,omitempty"`
	TopK     int                  `json:"top_k"`
	Filter   model.MetadataFilter `json:"filter,omitempty"`
	MinScore float32              `json:"min_score,omitempty"`
}

// VectorSearchResult encapsulates a single nearest-neighbor match returned by VectorStore.
// ContentMarkdown carries the chunk markdown from the persistent point when available.
type VectorSearchResult struct {
	ID              string               `json:"id"`
	Score           float32              `json:"score"`
	ContentMarkdown string               `json:"content_markdown,omitempty"`
	Metadata        model.VectorMetadata `json:"metadata"`
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
