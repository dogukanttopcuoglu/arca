package model

// Supported chunk content types.
const (
	ContentTypeParagraph = "paragraph"
	ContentTypeTable     = "table"
	ContentTypeCode      = "code"
	ContentTypeList      = "list"
	ContentTypeEquation  = "equation"
	ContentTypeFigure    = "figure"
)

// SourceOffset defines character start and end index bounds in source text.
type SourceOffset struct {
	StartChar int `json:"start_char"`
	EndChar   int `json:"end_char"`
}

// KnowledgeChunk represents a discrete, semantically intact section segment with provenance and parent-child hierarchy links.
type KnowledgeChunk struct {
	ChunkID          string           `json:"chunk_id"`
	ParentChunkID    *string          `json:"parent_chunk_id"`
	ChildChunkIDs    []string         `json:"child_chunk_ids"`
	PreviousChunkID  *string          `json:"previous_chunk_id,omitempty"`
	NextChunkID      *string          `json:"next_chunk_id,omitempty"`
	ChunkOrder       int              `json:"chunk_order"`
	DocumentID       string           `json:"document_id"`
	SectionPath      string           `json:"section_path"`
	HeadingLevel     int              `json:"heading_level"`
	PageNumbers      []int            `json:"page_numbers"`
	ContentMarkdown  string           `json:"content_markdown"`
	TokenEstimate    int              `json:"token_estimate"`
	CharacterCount   int              `json:"character_count"`
	Citations        []Citation       `json:"citations,omitempty"`
	SourceOffsets    SourceOffset     `json:"source_offsets"`
	ContentType      string           `json:"content_type"`
	SemanticCategory SemanticCategory `json:"semantic_category,omitempty"`
	ContentHash      string           `json:"content_hash,omitempty"`
	Fingerprint      string           `json:"fingerprint,omitempty"`
	IsOversized      bool             `json:"is_oversized,omitempty"`
	Keywords         []Keyword        `json:"keywords,omitempty"`
	Entities         []EntityMention  `json:"entities,omitempty"`
	Concepts         []Concept        `json:"concepts,omitempty"`
	Relations        []Relation       `json:"relations,omitempty"`
	Summary          *Summary         `json:"summary,omitempty"`
}
