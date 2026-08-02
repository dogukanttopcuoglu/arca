package seam

import (
	"fmt"
	"sort"

	indexingmodel "arca/internal/indexing/model"
)

// SearchResult isolates RAG consumers from raw vector database matches.
type SearchResult struct {
	ChunkID         string                       `json:"chunk_id"`
	ContentMarkdown string                       `json:"content_markdown,omitempty"`
	Score           float32                      `json:"score"`
	Metadata        indexingmodel.VectorMetadata `json:"metadata"`
}

// Validate verifies structural invariants for SearchResult.
func (r SearchResult) Validate() error {
	if r.ChunkID == "" {
		return fmt.Errorf("SearchResult ChunkID cannot be empty")
	}
	if err := r.Metadata.Validate(); err != nil {
		return fmt.Errorf("invalid SearchResult metadata: %w", err)
	}
	return nil
}

// SortResultsByScore sorts SearchResults in descending order by relevance score.
func SortResultsByScore(results []SearchResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
}
