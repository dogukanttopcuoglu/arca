package store

import (
	"crypto/sha256"
	"fmt"

	"arca/internal/indexing/model"
)

// CalculatePointID generates a deterministic, stable Point ID across content revisions: SHA256(DocumentID:SectionPath:ChunkOrder).
func CalculatePointID(documentID, sectionPath string, chunkOrder int) string {
	raw := fmt.Sprintf("%s:%s:%d", documentID, sectionPath, chunkOrder)
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h)
}

// VectorPoint represents a stored vector point containing stable Point ID, embedding vector, and typed VectorMetadata.
type VectorPoint struct {
	ID       string               `json:"id"`
	Vector   []float32            `json:"vector"`
	Metadata model.VectorMetadata `json:"metadata"`
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
