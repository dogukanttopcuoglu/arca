package store

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"

	"arca/internal/indexing/model"
	"arca/internal/indexing/sparse"
)

// InMemoryVectorStore is a thread-safe in-memory adapter for VectorStore using exact Cosine Similarity.
type InMemoryVectorStore struct {
	mu     sync.RWMutex
	points map[string]VectorPoint
}

// NewInMemoryVectorStore constructs an InMemoryVectorStore instance.
func NewInMemoryVectorStore() *InMemoryVectorStore {
	return &InMemoryVectorStore{
		points: make(map[string]VectorPoint),
	}
}

// Health checks in-memory store status.
func (s *InMemoryVectorStore) Health(ctx context.Context) error {
	return nil
}

// Points returns the total number of stored vector points (diagnostics helper).
func (s *InMemoryVectorStore) Points() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.points)
}

// UpsertPoints inserts or updates vector points in place based on Point ID.
func (s *InMemoryVectorStore) UpsertPoints(ctx context.Context, points []VectorPoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, pt := range points {
		if err := pt.Validate(); err != nil {
			return err
		}
		s.points[pt.ID] = pt
	}
	return nil
}

// SearchVector executes nearest-neighbor Cosine Similarity search with MetadataFilter.
func (s *InMemoryVectorStore) SearchVector(ctx context.Context, query VectorSearchQuery) ([]VectorSearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	topK := query.TopK
	if topK <= 0 {
		topK = 10
	}

	var results []VectorSearchResult

	for _, pt := range s.points {
		if !matchesFilter(pt.ID, pt.Metadata, query.Filter) {
			continue
		}

		score := float32(0.0)
		switch {
		case query.Sparse != nil:
			if pt.Sparse != nil {
				score = sparseDotProduct(query.Sparse, pt.Sparse)
			}
		case len(query.Vector) > 0:
			score = cosineSimilarity(query.Vector, pt.Vector)
		}
		if query.MinScore > 0 && score < query.MinScore {
			continue
		}

		results = append(results, VectorSearchResult{
			ID:              pt.ID,
			Score:           score,
			ContentMarkdown: pt.ContentMarkdown,
			Metadata:        pt.Metadata,
		})
	}

	// Sort results descending by score
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

// ListPoints enumerates all stored points matching the filter without ranking.
func (s *InMemoryVectorStore) ListPoints(ctx context.Context, filter model.MetadataFilter) ([]VectorPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var points []VectorPoint
	for id, pt := range s.points {
		if matchesFilter(id, pt.Metadata, filter) {
			points = append(points, pt)
		}
	}
	return points, nil
}

// Delete removes vector points matching the given MetadataFilter.
func (s *InMemoryVectorStore) Delete(ctx context.Context, filter model.MetadataFilter) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, pt := range s.points {
		if matchesFilter(id, pt.Metadata, filter) {
			delete(s.points, id)
		}
	}
	return nil
}

func matchesFilter(id string, meta model.VectorMetadata, filter model.MetadataFilter) bool {
	if len(filter.PointIDs) > 0 {
		found := false
		for _, ptID := range filter.PointIDs {
			if id == ptID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(filter.DocumentIDs) > 0 {
		found := false
		for _, docID := range filter.DocumentIDs {
			if meta.DocumentID == docID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(filter.ChunkIDs) > 0 {
		found := false
		for _, chkID := range filter.ChunkIDs {
			if meta.ChunkID == chkID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(filter.PageNumbers) > 0 {
		found := false
		for _, reqPage := range filter.PageNumbers {
			for _, ptPage := range meta.PageNumbers {
				if reqPage == ptPage {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}

	if filter.SectionPathPrefix != "" {
		if !strings.HasPrefix(meta.SectionPath, filter.SectionPathPrefix) {
			return false
		}
	}

	return true
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA <= 0 || normB <= 0 {
		return 0.0
	}

	similarity := dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
	return float32(similarity)
}

// sparseDotProduct computes the dot product over shared sparse indices.
func sparseDotProduct(a, b *sparse.SparseVector) float32 {
	// Both vectors are index-sorted by construction; merge linearly.
	i, j := 0, 0
	var sum float64
	for i < len(a.Indices) && j < len(b.Indices) {
		switch {
		case a.Indices[i] < b.Indices[j]:
			i++
		case a.Indices[i] > b.Indices[j]:
			j++
		default:
			sum += float64(a.Values[i]) * float64(b.Values[j])
			i++
			j++
		}
	}
	return float32(sum)
}
