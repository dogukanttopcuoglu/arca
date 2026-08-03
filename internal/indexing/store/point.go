package store

import (
	"crypto/sha256"
	"fmt"

	"arca/internal/indexing/model"
)

// CalculatePointID generates a deterministic, stable Point ID across content revisions: SHA256(DocumentID:SectionPath:ChunkOrder).
// The digest is formatted as a canonical UUID (version 5, RFC 4122 variant) so the
// ID is valid for Qdrant PointId (UUID) while remaining stable for differential indexing.
func CalculatePointID(documentID, sectionPath string, chunkOrder int) string {
	raw := fmt.Sprintf("%s:%s:%d", documentID, sectionPath, chunkOrder)
	h := sha256.Sum256([]byte(raw))
	h[6] = (h[6] & 0x0f) | 0x50
	h[8] = (h[8] & 0x3f) | 0x80
	hexstr := fmt.Sprintf("%x", h[:16])
	return hexstr[0:8] + "-" + hexstr[8:12] + "-" + hexstr[12:16] + "-" + hexstr[16:20] + "-" + hexstr[20:32]
}

// VectorPoint represents a stored vector point containing stable Point ID, embedding vector,
// chunk markdown content, and typed VectorMetadata.
type VectorPoint struct {
	ID              string               `json:"id"`
	Vector          []float32            `json:"vector"`
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
