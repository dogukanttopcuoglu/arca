package seam

import (
	"context"
)

// Retriever defines the clean domain interface seam for retrieving relevant knowledge chunks for RAG queries.
type Retriever interface {
	// Retrieve searches for relevant KnowledgeChunks matching query text and filters.
	Retrieve(ctx context.Context, query RetrievalQuery) ([]SearchResult, error)
}
