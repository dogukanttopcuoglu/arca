package model

import (
	"fmt"
	"time"

	pdfmodel "arca/internal/pdfinspector/model"
)

// MetadataFilter represents a strongly-typed, database-agnostic domain filtering abstraction across ingestion and retrieval.
type MetadataFilter struct {
	DocumentIDs       []string   `json:"document_ids,omitempty"`
	ChunkIDs          []string   `json:"chunk_ids,omitempty"`
	PageNumbers       []int      `json:"page_numbers,omitempty"`
	ContentTypes      []string   `json:"content_types,omitempty"`
	SectionPathPrefix string     `json:"section_path_prefix,omitempty"`
	IndexedAfter      *time.Time `json:"indexed_after,omitempty"`
}

// Validate verifies structural invariants for MetadataFilter.
func (f MetadataFilter) Validate() error {
	for _, pg := range f.PageNumbers {
		if pg < 1 {
			return fmt.Errorf("invalid page number in MetadataFilter: %d (must be >= 1)", pg)
		}
	}
	for _, ct := range f.ContentTypes {
		switch ct {
		case pdfmodel.ContentTypeParagraph, pdfmodel.ContentTypeTable, pdfmodel.ContentTypeCode, pdfmodel.ContentTypeList, pdfmodel.ContentTypeEquation, pdfmodel.ContentTypeFigure:
			// Valid
		default:
			return fmt.Errorf("unsupported content_type in MetadataFilter: %q", ct)
		}
	}
	return nil
}
