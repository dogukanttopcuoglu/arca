package store

import (
	"fmt"

	graphmodel "arca/internal/graph/model"
	"arca/internal/indexing/model"
	"arca/internal/indexing/sparse"
)

// CalculatePointID generates a deterministic, stable Point ID across content
// revisions: SHA256(DocumentID:SectionPath:ChunkOrder) formatted as a UUID
// (RFC 4122 variant bits). It delegates the UUID formatting to the single
// shared helper in internal/graph/model.
func CalculatePointID(documentID, sectionPath string, chunkOrder int) string {
	raw := fmt.Sprintf("%s:%s:%d", documentID, sectionPath, chunkOrder)
	return graphmodel.CalculatePointID(raw)
}

// VectorPoint represents a stored vector point containing stable Point ID, dense embedding
// vector, optional sparse vector, chunk markdown content, and typed VectorMetadata.
type VectorPoint struct {
	ID              string               `json:"id"`
	Vector          []float32            `json:"vector"`
	Sparse          *sparse.SparseVector `json:"sparse,omitempty"`
	ContentMarkdown string               `json:"content_markdown,omitempty"`
	Metadata        model.VectorMetadata `json:"metadata"`
}

// Validate verifies structural invariants for VectorPoint.
func (p VectorPoint) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("vector point ID cannot be empty")
	}
	if len(p.Vector) == 0 {
		return fmt.Errorf("vector point vector slice cannot be empty")
	}
	if err := p.Metadata.Validate(); err != nil {
		return fmt.Errorf("invalid vector point metadata: %w", err)
	}
	return nil
}
