package seam

import (
	"fmt"

	indexingmodel "arca/internal/indexing/model"
)

// RetrievalMode specifies the underlying search mechanism enum.
type RetrievalMode int

const (
	RetrievalDense RetrievalMode = iota
	RetrievalSparse
	RetrievalHybrid
)

// String returns human-readable representation of RetrievalMode.
func (m RetrievalMode) String() string {
	switch m {
	case RetrievalDense:
		return "Dense"
	case RetrievalSparse:
		return "Sparse"
	case RetrievalHybrid:
		return "Hybrid"
	default:
		return "Unknown"
	}
}

// RetrievalQuery encapsulates parameters for knowledge retrieval requests.
type RetrievalQuery struct {
	QueryText string                       `json:"query_text"`
	TopK      int                          `json:"top_k"`
	Mode      RetrievalMode                `json:"mode"`
	Filter    indexingmodel.MetadataFilter `json:"filter,omitempty"`
	MinScore  float32                      `json:"min_score,omitempty"`
}

// Validate verifies structural invariants for RetrievalQuery.
func (q RetrievalQuery) Validate() error {
	if q.QueryText == "" {
		return fmt.Errorf("QueryText cannot be empty")
	}
	if err := q.Filter.Validate(); err != nil {
		return fmt.Errorf("invalid MetadataFilter in query: %w", err)
	}
	return nil
}

// Normalize applies sensible defaults (e.g. TopK=10 if <= 0).
func (q *RetrievalQuery) Normalize() {
	if q.TopK <= 0 {
		q.TopK = 10
	}
}
