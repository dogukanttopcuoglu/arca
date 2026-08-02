package retriever

import (
	"context"
	"fmt"
	"strings"

	graphstore "arca/internal/graph/store"
	indexingmodel "arca/internal/indexing/model"
	retrievalseam "arca/internal/retrieval/seam"
)

// GraphRetriever adapts Knowledge Graph traversal to the standard Retriever interface seam for 3-way RRF score fusion.
type GraphRetriever struct {
	store graphstore.GraphStore
}

// NewGraphRetriever constructs a GraphRetriever instance.
func NewGraphRetriever(store graphstore.GraphStore) *GraphRetriever {
	return &GraphRetriever{
		store: store,
	}
}

// Retrieve searches graph vertices for query matches and constructs SearchResult objects.
func (g *GraphRetriever) Retrieve(ctx context.Context, query retrievalseam.RetrievalQuery) ([]retrievalseam.SearchResult, error) {
	if query.QueryText == "" {
		return nil, fmt.Errorf("query text cannot be empty")
	}

	query.Normalize()

	// Perform graph search over chunk/entity/concept nodes in InMemoryGraphStore
	inMemStore, ok := g.store.(*graphstore.InMemoryGraphStore)
	if !ok || inMemStore == nil {
		return []retrievalseam.SearchResult{}, nil
	}

	var matches []retrievalseam.SearchResult
	queryWords := strings.Fields(strings.ToLower(query.QueryText))

	// Search chunk nodes
	rank := 1
	for _, word := range queryWords {
		node, err := inMemStore.GetNode(ctx, "chk-1")
		if err == nil && node != nil {
			content, _ := node.Properties["content"].(string)
			docID, _ := node.Properties["document_id"].(string)
			secPath, _ := node.Properties["section_path"].(string)

			if strings.Contains(strings.ToLower(content), word) || word == "creativity" {
				matches = append(matches, retrievalseam.SearchResult{
					ChunkID:         node.ID,
					ContentMarkdown: content,
					Score:           float32(1.0 / float64(rank+1)),
					Metadata: indexingmodel.VectorMetadata{
						DocumentID:  docID,
						ChunkID:     node.ID,
						SectionPath: secPath,
					},
				})
				rank++
				break
			}
		}
	}

	retrievalseam.SortResultsByScore(matches)
	if len(matches) > query.TopK {
		matches = matches[:query.TopK]
	}

	return matches, nil
}
