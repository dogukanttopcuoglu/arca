package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Sentinel resiliency errors.
var (
	ErrEncryptedDocument = errors.New("ENCRYPTED_DOCUMENT: PDF document is encrypted or password protected")
	ErrInvalidDocument   = errors.New("INVALID_DOCUMENT: PDF file structure is invalid or unreadable")
)

// Supported schema versions.
const (
	SchemaVersionV1 = "1.0.0"
)

// Execution diagnostics status constants.
const (
	StatusSuccess        = "success"
	StatusPartialSuccess = "partial_success"
	StatusFailed         = "failed"
)

// Supported chunk content types.
const (
	ContentTypeParagraph = "paragraph"
	ContentTypeTable     = "table"
	ContentTypeCode      = "code"
	ContentTypeList      = "list"
	ContentTypeEquation  = "equation"
	ContentTypeFigure    = "figure"
)

// SemanticCategory defines typed constants for chunk semantic classification.
type SemanticCategory string

const (
	SemanticNarrative  SemanticCategory = "narrative"
	SemanticDefinition SemanticCategory = "definition"
	SemanticProcedure  SemanticCategory = "procedure"
	SemanticReference  SemanticCategory = "reference"
	SemanticExample    SemanticCategory = "example"
	SemanticWarning    SemanticCategory = "warning"
	SemanticCode       SemanticCategory = "code"
	SemanticTable      SemanticCategory = "table"
	SemanticEquation   SemanticCategory = "equation"
	SemanticFigure     SemanticCategory = "figure"
)

// Validatable describes models capable of self-validation for structural correctness and domain invariants.
type Validatable interface {
	Validate() error
}

// KeywordSource defines the typed provenance of an extracted keyword.
type KeywordSource string

const (
	KeywordSourceRuleBased KeywordSource = "rule_based"
	KeywordSourceLLM       KeywordSource = "llm"
	KeywordSourceHybrid    KeywordSource = "hybrid"
)

// Keyword represents structured semantic keyword metadata.
type Keyword struct {
	Value    string        `json:"value"`
	Score    float64       `json:"score"`
	Source   KeywordSource `json:"source"`
	ChunkIDs []string      `json:"chunk_ids,omitempty"`
}

// EntityType defines typed classification for named entities.
type EntityType string

const (
	EntityTypePerson       EntityType = "person"
	EntityTypeOrganization EntityType = "organization"
	EntityTypeLocation     EntityType = "location"
	EntityTypeProduct      EntityType = "product"
	EntityTypeEvent        EntityType = "event"
	EntityTypeMisc         EntityType = "miscellaneous"
)

// EntityMention represents a surface text occurrence of a typed entity.
type EntityMention struct {
	Text       string     `json:"text"`
	Type       EntityType `json:"type"`
	ChunkID    string     `json:"chunk_id"`
	Confidence float64    `json:"confidence"`
}

// Entity represents a document-level aggregated entity record.
type Entity struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Type     EntityType      `json:"type"`
	Aliases  []string        `json:"aliases,omitempty"`
	Mentions []EntityMention `json:"mentions,omitempty"`
	Score    float64         `json:"score"`
}

// DocumentMetadata represents administrative, technical, and structural metadata extracted from a PDF document.
type DocumentMetadata struct {
	Title            string    `json:"title,omitempty"`
	Author           string    `json:"author,omitempty"`
	Creator          string    `json:"creator,omitempty"`
	Producer         string    `json:"producer,omitempty"`
	CreationDate     time.Time `json:"creationDate,omitempty"`
	ModificationDate time.Time `json:"modificationDate,omitempty"`
	PageCount        int       `json:"pageCount"`
	PageDimensions   string    `json:"pageDimensions,omitempty"`
	Fonts            []string  `json:"fonts,omitempty"`
	Encrypted        bool      `json:"encrypted"`
	Searchable       bool      `json:"searchable"`
	PDFType          string    `json:"pdfType,omitempty"`
	Language         string    `json:"language,omitempty"`
	Keywords         []Keyword `json:"keywords,omitempty"`
	Entities         []Entity  `json:"entities,omitempty"`
}

// Validate verifies structural correctness for DocumentMetadata.
func (m DocumentMetadata) Validate() error {
	if m.PageCount < 0 {
		return fmt.Errorf("invalid pageCount: %d (must be >= 0)", m.PageCount)
	}
	return nil
}

// SemanticNode represents a heading/section node in the document hierarchy tree.
type SemanticNode struct {
	ID          string         `json:"id"`
	Heading     string         `json:"heading"`
	Level       int            `json:"level"`
	PageNumbers []int          `json:"pageNumbers"`
	Children    []SemanticNode `json:"children,omitempty"`
}

// Validate verifies structural invariants for a SemanticNode.
func (n SemanticNode) Validate() error {
	if n.ID == "" {
		return fmt.Errorf("semantic node ID cannot be empty")
	}
	if n.Level < 1 {
		return fmt.Errorf("semantic node level must be >= 1, got %d", n.Level)
	}
	for _, pg := range n.PageNumbers {
		if pg < 1 {
			return fmt.Errorf("invalid page number in semantic node %s: %d", n.ID, pg)
		}
	}
	for _, child := range n.Children {
		if err := child.Validate(); err != nil {
			return fmt.Errorf("invalid child node in %s: %w", n.ID, err)
		}
	}
	return nil
}

// SemanticTree represents the reconstructed hierarchical tree of document sections.
type SemanticTree struct {
	RootNodes []SemanticNode `json:"rootNodes"`
}

// Validate verifies structural invariants for SemanticTree.
func (t SemanticTree) Validate() error {
	for _, root := range t.RootNodes {
		if err := root.Validate(); err != nil {
			return fmt.Errorf("invalid root node: %w", err)
		}
	}
	return nil
}

// PageMap maps an individual PDF page number to its extracted raw Markdown text.
type PageMap struct {
	PageNumber int    `json:"pageNumber"`
	Markdown   string `json:"markdown"`
}

// Validate verifies page map structural invariants.
func (pm PageMap) Validate() error {
	if pm.PageNumber < 1 {
		return fmt.Errorf("invalid pageNumber in PageMap: %d (must be >= 1)", pm.PageNumber)
	}
	return nil
}

// DocumentContent encapsulates extracted complete text and per-page layout maps.
type DocumentContent struct {
	Markdown string    `json:"markdown"`
	PageMap  []PageMap `json:"pageMap"`
}

// Validate verifies document content structural invariants.
func (c DocumentContent) Validate() error {
	for _, pm := range c.PageMap {
		if err := pm.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// SourceOffset defines character start and end index bounds in source text.
type SourceOffset struct {
	StartChar int `json:"start_char"`
	EndChar   int `json:"end_char"`
}

// Validate verifies source offset bounds invariants.
func (so SourceOffset) Validate() error {
	if so.StartChar < 0 {
		return fmt.Errorf("invalid start_char: %d (must be >= 0)", so.StartChar)
	}
	if so.EndChar < 0 {
		return fmt.Errorf("invalid end_char: %d (must be >= 0)", so.EndChar)
	}
	if so.StartChar > so.EndChar {
		return fmt.Errorf("invalid source offset: start_char %d > end_char %d", so.StartChar, so.EndChar)
	}
	return nil
}

// AssetType represents typed classification for extracted non-prose document assets.
type AssetType string

const (
	AssetTypeTable      AssetType = "table"
	AssetTypeFigure     AssetType = "figure"
	AssetTypeCodeBlock  AssetType = "code_block"
	AssetTypeEquation   AssetType = "equation"
	AssetTypeCitation   AssetType = "citation"
)

// IsValid verifies whether AssetType is a recognized type.
func (a AssetType) IsValid() bool {
	switch a {
	case AssetTypeTable, AssetTypeFigure, AssetTypeCodeBlock, AssetTypeEquation, AssetTypeCitation:
		return true
	default:
		return false
	}
}

// CitationType defines typed classifications for document references.
type CitationType string

const (
	CitationTypeInline       CitationType = "inline"
	CitationTypeFootnote     CitationType = "footnote"
	CitationTypeBibliography CitationType = "bibliography"
	CitationTypeAttribution  CitationType = "attribution"
)

// IsValid verifies whether CitationType is a recognized citation classification.
func (c CitationType) IsValid() bool {
	switch c {
	case CitationTypeInline, CitationTypeFootnote, CitationTypeBibliography, CitationTypeAttribution:
		return true
	default:
		return false
	}
}

// ExtractionErrorSeverity defines error log levels for asset extraction processing.
type ExtractionErrorSeverity string

const (
	SeverityWarning  ExtractionErrorSeverity = "warning"
	SeverityCritical ExtractionErrorSeverity = "critical"
)

// SourceLocation defines line and character position bounds in source text.
type SourceLocation struct {
	StartOffset int `json:"startOffset"`
	EndOffset   int `json:"endOffset"`
	StartLine   int `json:"startLine"`
	EndLine     int `json:"endLine"`
}

// Validate verifies SourceLocation invariants.
func (loc SourceLocation) Validate() error {
	if loc.StartOffset < 0 {
		return fmt.Errorf("invalid startOffset: %d (must be >= 0)", loc.StartOffset)
	}
	if loc.EndOffset < loc.StartOffset {
		return fmt.Errorf("invalid source location: startOffset %d > endOffset %d", loc.StartOffset, loc.EndOffset)
	}
	if loc.StartLine < 0 {
		return fmt.Errorf("invalid startLine: %d (must be >= 0)", loc.StartLine)
	}
	if loc.EndLine < loc.StartLine {
		return fmt.Errorf("invalid line numbers: startLine %d > endLine %d", loc.StartLine, loc.EndLine)
	}
	return nil
}

// PageContext encapsulates page resolution mapping for extracted document elements.
type PageContext struct {
	PrimaryPage int   `json:"primaryPage"`
	Pages       []int `json:"pages"`
}

// AssetMetadata aggregates shared identity, spatial, section, and provenance metadata across document assets.
type AssetMetadata struct {
	ID              string         `json:"id"`
	AssetType       AssetType      `json:"assetType"`
	PageNumber      int            `json:"pageNumber"`
	PageNumbers     []int          `json:"pageNumbers,omitempty"`
	SectionPath     string         `json:"sectionPath,omitempty"`
	SourceLocation  SourceLocation `json:"sourceLocation"`
	RelatedChunkIDs []string       `json:"relatedChunkIds,omitempty"`
}

// Validate performs extraction-time validation of AssetMetadata invariants.
func (m AssetMetadata) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("asset ID cannot be empty")
	}
	if !m.AssetType.IsValid() {
		return fmt.Errorf("unsupported assetType: %q", m.AssetType)
	}
	if err := m.SourceLocation.Validate(); err != nil {
		return err
	}
	return nil
}

// ValidateComplete performs post-processing validation requiring resolved page numbers.
func (m AssetMetadata) ValidateComplete() error {
	if err := m.Validate(); err != nil {
		return err
	}
	pg := m.PageNumber
	if pg < 1 && len(m.PageNumbers) > 0 {
		pg = m.PageNumbers[0]
	}
	if pg < 1 {
		return fmt.Errorf("invalid page context in asset %s: primary page %d", m.ID, m.PageNumber)
	}
	return nil
}

// Asset defines the common interface implemented by all non-prose extracted document components.
type Asset interface {
	GetMetadata() AssetMetadata
}

// Table represents an extracted table structure.
type Table struct {
	AssetMetadata
	Caption string   `json:"caption,omitempty"`
	Content string   `json:"content"`
	Headers []string `json:"headers,omitempty"`
}

// GetMetadata implements Asset interface.
func (tbl Table) GetMetadata() AssetMetadata { return tbl.AssetMetadata }

// Validate verifies Table invariants.
func (tbl Table) Validate() error {
	return tbl.AssetMetadata.Validate()
}

// Figure represents an extracted graphic, diagram, or picture.
type Figure struct {
	AssetMetadata
	Caption string `json:"caption,omitempty"`
	URI     string `json:"uri,omitempty"`
}

// GetMetadata implements Asset interface.
func (fig Figure) GetMetadata() AssetMetadata { return fig.AssetMetadata }

// Validate verifies Figure invariants.
func (fig Figure) Validate() error {
	return fig.AssetMetadata.Validate()
}

// CodeBlock represents an extracted code listing.
type CodeBlock struct {
	AssetMetadata
	Language string `json:"language,omitempty"`
	Content  string `json:"content"`
}

// GetMetadata implements Asset interface.
func (cb CodeBlock) GetMetadata() AssetMetadata { return cb.AssetMetadata }

// Validate verifies CodeBlock invariants.
func (cb CodeBlock) Validate() error {
	return cb.AssetMetadata.Validate()
}

// Equation represents an extracted LaTeX or mathematical formula.
type Equation struct {
	AssetMetadata
	LaTeX string `json:"latex"`
}

// GetMetadata implements Asset interface.
func (eq Equation) GetMetadata() AssetMetadata { return eq.AssetMetadata }

// Validate verifies Equation invariants.
func (eq Equation) Validate() error {
	return eq.AssetMetadata.Validate()
}

// Citation represents an inline or bibliographic document reference.
type Citation struct {
	AssetMetadata
	RawText      string       `json:"rawText"`
	CitationType CitationType `json:"citationType,omitempty"`
}

// GetMetadata implements Asset interface.
func (cit Citation) GetMetadata() AssetMetadata { return cit.AssetMetadata }

// Validate verifies Citation invariants.
func (cit Citation) Validate() error {
	return cit.AssetMetadata.Validate()
}

// ExtractionWarning records non-fatal asset processing warnings during ingestion.
type ExtractionWarning struct {
	AssetID        string                  `json:"assetId,omitempty"`
	Message        string                  `json:"message"`
	Severity       ExtractionErrorSeverity `json:"severity,omitempty"`
	SourceLocation SourceLocation          `json:"sourceLocation,omitempty"`
}

// AssetReference preserves original document ordering across heterogeneous asset types.
type AssetReference struct {
	ID             string         `json:"id"`
	AssetType      AssetType      `json:"assetType"`
	SourceLocation SourceLocation `json:"sourceLocation"`
}

// ExtractionStats holds processing metrics for asset extraction.
type ExtractionStats struct {
	TablesFound     int   `json:"tablesFound"`
	FiguresFound    int   `json:"figuresFound"`
	CodeBlocksFound int   `json:"codeBlocksFound"`
	EquationsFound  int   `json:"equationsFound"`
	CitationsFound  int   `json:"citationsFound"`
	WarningCount    int   `json:"warningCount"`
	DurationMs      int64 `json:"durationMs"`
}

// ExtractedAssets is an intermediate aggregation of raw sub-extractor outputs.
type ExtractedAssets struct {
	Tables     []Table     `json:"tables"`
	Figures    []Figure    `json:"figures"`
	CodeBlocks []CodeBlock `json:"codeBlocks"`
	Equations  []Equation  `json:"equations"`
	Citations  []Citation  `json:"citations"`
}

// Assets aggregates all extracted non-prose document components, warnings, stats, and document ordering references.
type Assets struct {
	Tables     []Table             `json:"tables"`
	Figures    []Figure            `json:"figures"`
	CodeBlocks []CodeBlock         `json:"codeBlocks"`
	Equations  []Equation          `json:"equations"`
	Citations  []Citation          `json:"citations"`
	Warnings   []ExtractionWarning `json:"warnings,omitempty"`
	Ordered    []AssetReference    `json:"ordered,omitempty"`
	Stats      ExtractionStats     `json:"stats"`
}

// Validate verifies Assets invariants.
func (a Assets) Validate() error {
	for _, tbl := range a.Tables {
		if err := tbl.Validate(); err != nil {
			return err
		}
	}
	for _, fig := range a.Figures {
		if err := fig.Validate(); err != nil {
			return err
		}
	}
	for _, cb := range a.CodeBlocks {
		if err := cb.Validate(); err != nil {
			return err
		}
	}
	for _, eq := range a.Equations {
		if err := eq.Validate(); err != nil {
			return err
		}
	}
	for _, cit := range a.Citations {
		if err := cit.Validate(); err != nil {
			return err
		}
	}
	return nil
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
}

// Validate verifies KnowledgeChunk invariants.
func (c KnowledgeChunk) Validate() error {
	if c.ChunkID == "" {
		return fmt.Errorf("chunk_id cannot be empty")
	}
	if c.HeadingLevel < 0 {
		return fmt.Errorf("heading_level must be >= 0, got %d", c.HeadingLevel)
	}
	if c.TokenEstimate < 0 {
		return fmt.Errorf("token_estimate must be >= 0, got %d", c.TokenEstimate)
	}
	if c.CharacterCount < 0 {
		return fmt.Errorf("character_count must be >= 0, got %d", c.CharacterCount)
	}
	switch c.ContentType {
	case ContentTypeParagraph, ContentTypeTable, ContentTypeCode, ContentTypeList, ContentTypeEquation, ContentTypeFigure:
		// Valid content types
	default:
		return fmt.Errorf("unsupported content_type: %q", c.ContentType)
	}
	for _, pg := range c.PageNumbers {
		if pg < 1 {
			return fmt.Errorf("invalid page number in chunk %s: %d", c.ChunkID, pg)
		}
	}
	if err := c.SourceOffsets.Validate(); err != nil {
		return fmt.Errorf("invalid source_offsets in chunk %s: %w", c.ChunkID, err)
	}
	for _, cit := range c.Citations {
		if err := cit.Validate(); err != nil {
			return fmt.Errorf("invalid citation in chunk %s: %w", c.ChunkID, err)
		}
	}
	return nil
}

// Diagnostics records execution metrics, warnings, errors, and degradation details.
type Diagnostics struct {
	Status           string   `json:"status"`
	ExtractionEngine string   `json:"extractionEngine"`
	ExtractionVer    string   `json:"extractionVersion"`
	ProcessingTimeMs int64    `json:"processingTimeMs"`
	Warnings         []string `json:"warnings"`
	Errors           []string `json:"errors"`
	SkippedPages     []int    `json:"skippedPages"`
	RetryCount       int      `json:"retryCount"`
}

// Validate verifies Diagnostics invariants.
func (d Diagnostics) Validate() error {
	switch d.Status {
	case StatusSuccess, StatusPartialSuccess, StatusFailed:
		// Valid status
	default:
		return fmt.Errorf("invalid diagnostics status: %q", d.Status)
	}
	if d.ProcessingTimeMs < 0 {
		return fmt.Errorf("invalid processingTimeMs: %d (must be >= 0)", d.ProcessingTimeMs)
	}
	if d.RetryCount < 0 {
		return fmt.Errorf("invalid retryCount: %d (must be >= 0)", d.RetryCount)
	}
	for _, pg := range d.SkippedPages {
		if pg < 1 {
			return fmt.Errorf("invalid page number in skippedPages: %d", pg)
		}
	}
	return nil
}

// PDFInspectionResult is the canonical pipeline intermediate representation for document ingestion across ARC services.
type PDFInspectionResult struct {
	SchemaVersion string           `json:"schemaVersion"`
	Document      DocumentMetadata `json:"document"`
	SemanticTree  SemanticTree     `json:"semanticTree"`
	Content       DocumentContent  `json:"content"`
	Chunks        []KnowledgeChunk `json:"chunks"`
	Assets        Assets           `json:"assets"`
	Diagnostics   Diagnostics      `json:"diagnostics"`
}

// RawExtractionResult represents the intermediate output returned by the Firecrawl PDF extraction microservice.
type RawExtractionResult struct {
	Markdown   string                 `json:"markdown"`
	JSONLayout map[string]interface{} `json:"json_layout"`
	Metadata   map[string]interface{} `json:"metadata"`
	OCRApplied bool                   `json:"ocr_applied"`
}

// NewPDFInspectionResult constructs a PDFInspectionResult with version stamping and empty non-nil slices.
func NewPDFInspectionResult() *PDFInspectionResult {
	return &PDFInspectionResult{
		SchemaVersion: SchemaVersionV1,
		Document: DocumentMetadata{
			Fonts: []string{},
		},
		SemanticTree: SemanticTree{
			RootNodes: []SemanticNode{},
		},
		Content: DocumentContent{
			PageMap: []PageMap{},
		},
		Chunks: []KnowledgeChunk{},
		Assets: Assets{
			Tables:     []Table{},
			Figures:    []Figure{},
			CodeBlocks: []CodeBlock{},
			Equations:  []Equation{},
			Citations:  []Citation{},
			Warnings:   []ExtractionWarning{},
			Ordered:    []AssetReference{},
		},
		Diagnostics: Diagnostics{
			Status:           StatusSuccess,
			ExtractionEngine: "firecrawl",
			ExtractionVer:    "1.0.0",
			Warnings:         []string{},
			Errors:           []string{},
			SkippedPages:     []int{},
			ProcessingTimeMs: 0,
			RetryCount:       0,
		},
	}
}

// Validate verifies structural correctness and domain invariants of PDFInspectionResult.
func (r *PDFInspectionResult) Validate() error {
	if r == nil {
		return fmt.Errorf("PDFInspectionResult cannot be nil")
	}
	if r.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("unsupported schemaVersion: %q (expected %q)", r.SchemaVersion, SchemaVersionV1)
	}
	if err := r.Document.Validate(); err != nil {
		return fmt.Errorf("invalid Document: %w", err)
	}
	if err := r.SemanticTree.Validate(); err != nil {
		return fmt.Errorf("invalid SemanticTree: %w", err)
	}
	if err := r.Content.Validate(); err != nil {
		return fmt.Errorf("invalid Content: %w", err)
	}
	if err := r.Assets.Validate(); err != nil {
		return fmt.Errorf("invalid Assets: %w", err)
	}
	if err := r.Diagnostics.Validate(); err != nil {
		return fmt.Errorf("invalid Diagnostics: %w", err)
	}

	chunkMap := make(map[string]bool)
	for _, chunk := range r.Chunks {
		if err := chunk.Validate(); err != nil {
			return fmt.Errorf("invalid KnowledgeChunk: %w", err)
		}
		chunkMap[chunk.ChunkID] = true
	}

	// Verify parent/child relationship integrity if parent_chunk_id or child_chunk_ids are set
	for _, chunk := range r.Chunks {
		if chunk.ParentChunkID != nil {
			if *chunk.ParentChunkID != "" && !chunkMap[*chunk.ParentChunkID] {
				// Note: Parent chunk ID must exist in chunks list if referenced within document scope
				return fmt.Errorf("chunk %s references missing parent_chunk_id %s", chunk.ChunkID, *chunk.ParentChunkID)
			}
		}
		for _, childID := range chunk.ChildChunkIDs {
			if !chunkMap[childID] {
				return fmt.Errorf("chunk %s references missing child_chunk_id %s", chunk.ChunkID, childID)
			}
		}
	}

	return nil
}

// ToJSON serializes PDFInspectionResult to JSON bytes.
func (r *PDFInspectionResult) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

// ToJSONIndent serializes PDFInspectionResult to formatted JSON bytes.
func (r *PDFInspectionResult) ToJSONIndent(prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(r, prefix, indent)
}

// PDFInspectionResultFromJSON deserializes JSON bytes into a PDFInspectionResult struct, populating default schema version and slices if empty.
func PDFInspectionResultFromJSON(data []byte) (*PDFInspectionResult, error) {
	res := NewPDFInspectionResult()
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(res); err != nil {
		return nil, fmt.Errorf("failed to decode PDFInspectionResult: %w", err)
	}

	if res.SchemaVersion == "" {
		res.SchemaVersion = SchemaVersionV1
	}

	// Guarantee slices are initialized (not nil) after unmarshaling
	if res.Document.Fonts == nil {
		res.Document.Fonts = []string{}
	}
	if res.SemanticTree.RootNodes == nil {
		res.SemanticTree.RootNodes = []SemanticNode{}
	}
	if res.Content.PageMap == nil {
		res.Content.PageMap = []PageMap{}
	}
	if res.Chunks == nil {
		res.Chunks = []KnowledgeChunk{}
	}
	for i := range res.Chunks {
		if res.Chunks[i].ChildChunkIDs == nil {
			res.Chunks[i].ChildChunkIDs = []string{}
		}
		if res.Chunks[i].PageNumbers == nil {
			res.Chunks[i].PageNumbers = []int{}
		}
		if res.Chunks[i].Citations == nil {
			res.Chunks[i].Citations = []Citation{}
		}
	}
	if res.Assets.Tables == nil {
		res.Assets.Tables = []Table{}
	}
	if res.Assets.Figures == nil {
		res.Assets.Figures = []Figure{}
	}
	if res.Assets.CodeBlocks == nil {
		res.Assets.CodeBlocks = []CodeBlock{}
	}
	if res.Assets.Equations == nil {
		res.Assets.Equations = []Equation{}
	}
	if res.Assets.Citations == nil {
		res.Assets.Citations = []Citation{}
	}
	if res.Assets.Warnings == nil {
		res.Assets.Warnings = []ExtractionWarning{}
	}
	if res.Assets.Ordered == nil {
		res.Assets.Ordered = []AssetReference{}
	}
	if res.Diagnostics.Warnings == nil {
		res.Diagnostics.Warnings = []string{}
	}
	if res.Diagnostics.Errors == nil {
		res.Diagnostics.Errors = []string{}
	}
	if res.Diagnostics.SkippedPages == nil {
		res.Diagnostics.SkippedPages = []int{}
	}

	return res, nil
}

// MarshalJSON implements json.Marshaler for PDFInspectionResult ensuring version stamping and slice initializations.
func (r *PDFInspectionResult) MarshalJSON() ([]byte, error) {
	type Alias PDFInspectionResult
	aux := (*Alias)(r)

	if aux.SchemaVersion == "" {
		aux.SchemaVersion = SchemaVersionV1
	}
	if aux.Document.Fonts == nil {
		aux.Document.Fonts = []string{}
	}
	if aux.SemanticTree.RootNodes == nil {
		aux.SemanticTree.RootNodes = []SemanticNode{}
	}
	if aux.Content.PageMap == nil {
		aux.Content.PageMap = []PageMap{}
	}
	if aux.Chunks == nil {
		aux.Chunks = []KnowledgeChunk{}
	}
	if aux.Assets.Tables == nil {
		aux.Assets.Tables = []Table{}
	}
	if aux.Assets.Figures == nil {
		aux.Assets.Figures = []Figure{}
	}
	if aux.Assets.CodeBlocks == nil {
		aux.Assets.CodeBlocks = []CodeBlock{}
	}
	if aux.Assets.Equations == nil {
		aux.Assets.Equations = []Equation{}
	}
	if aux.Assets.Citations == nil {
		aux.Assets.Citations = []Citation{}
	}
	if aux.Assets.Warnings == nil {
		aux.Assets.Warnings = []ExtractionWarning{}
	}
	if aux.Assets.Ordered == nil {
		aux.Assets.Ordered = []AssetReference{}
	}
	if aux.Diagnostics.Warnings == nil {
		aux.Diagnostics.Warnings = []string{}
	}
	if aux.Diagnostics.Errors == nil {
		aux.Diagnostics.Errors = []string{}
	}
	if aux.Diagnostics.SkippedPages == nil {
		aux.Diagnostics.SkippedPages = []int{}
	}

	return json.Marshal(aux)
}

// UnmarshalJSON implements json.Unmarshaler for PDFInspectionResult ensuring version stamping and non-nil slices.
func (r *PDFInspectionResult) UnmarshalJSON(data []byte) error {
	type Alias PDFInspectionResult
	aux := (*Alias)(r)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if r.SchemaVersion == "" {
		r.SchemaVersion = SchemaVersionV1
	}
	if r.Document.Fonts == nil {
		r.Document.Fonts = []string{}
	}
	if r.SemanticTree.RootNodes == nil {
		r.SemanticTree.RootNodes = []SemanticNode{}
	}
	if r.Content.PageMap == nil {
		r.Content.PageMap = []PageMap{}
	}
	if r.Chunks == nil {
		r.Chunks = []KnowledgeChunk{}
	}
	for i := range r.Chunks {
		if r.Chunks[i].ChildChunkIDs == nil {
			r.Chunks[i].ChildChunkIDs = []string{}
		}
		if r.Chunks[i].PageNumbers == nil {
			r.Chunks[i].PageNumbers = []int{}
		}
		if r.Chunks[i].Citations == nil {
			r.Chunks[i].Citations = []Citation{}
		}
	}
	if r.Assets.Tables == nil {
		r.Assets.Tables = []Table{}
	}
	if r.Assets.Figures == nil {
		r.Assets.Figures = []Figure{}
	}
	if r.Assets.CodeBlocks == nil {
		r.Assets.CodeBlocks = []CodeBlock{}
	}
	if r.Assets.Equations == nil {
		r.Assets.Equations = []Equation{}
	}
	if r.Assets.Citations == nil {
		r.Assets.Citations = []Citation{}
	}
	if r.Assets.Warnings == nil {
		r.Assets.Warnings = []ExtractionWarning{}
	}
	if r.Assets.Ordered == nil {
		r.Assets.Ordered = []AssetReference{}
	}
	if r.Diagnostics.Warnings == nil {
		r.Diagnostics.Warnings = []string{}
	}
	if r.Diagnostics.Errors == nil {
		r.Diagnostics.Errors = []string{}
	}
	if r.Diagnostics.SkippedPages == nil {
		r.Diagnostics.SkippedPages = []int{}
	}
	return nil
}

// DeepCopy creates a complete deep copy of PDFInspectionResult with isolated memory allocations.
func (r *PDFInspectionResult) DeepCopy() *PDFInspectionResult {
	if r == nil {
		return nil
	}
	cp := *r

	// Deep copy Document
	if r.Document.Fonts != nil {
		cp.Document.Fonts = make([]string, len(r.Document.Fonts))
		copy(cp.Document.Fonts, r.Document.Fonts)
	}

	// Deep copy SemanticTree
	cp.SemanticTree = r.SemanticTree.DeepCopy()

	// Deep copy Content
	if r.Content.PageMap != nil {
		cp.Content.PageMap = make([]PageMap, len(r.Content.PageMap))
		copy(cp.Content.PageMap, r.Content.PageMap)
	}

	// Deep copy Chunks
	if r.Chunks != nil {
		cp.Chunks = make([]KnowledgeChunk, len(r.Chunks))
		for i, chk := range r.Chunks {
			cp.Chunks[i] = chk.DeepCopy()
		}
	}

	// Deep copy Assets
	cp.Assets = r.Assets.DeepCopy()

	// Deep copy Diagnostics
	cp.Diagnostics = r.Diagnostics.DeepCopy()

	return &cp
}

// Clone is an alias for DeepCopy.
func (r *PDFInspectionResult) Clone() *PDFInspectionResult {
	return r.DeepCopy()
}

// DeepCopy creates a deep copy of SemanticTree.
func (t SemanticTree) DeepCopy() SemanticTree {
	cp := SemanticTree{}
	if t.RootNodes != nil {
		cp.RootNodes = make([]SemanticNode, len(t.RootNodes))
		for i, n := range t.RootNodes {
			cp.RootNodes[i] = n.DeepCopy()
		}
	}
	return cp
}

// DeepCopy creates a deep copy of SemanticNode.
func (n SemanticNode) DeepCopy() SemanticNode {
	cp := n
	if n.PageNumbers != nil {
		cp.PageNumbers = make([]int, len(n.PageNumbers))
		copy(cp.PageNumbers, n.PageNumbers)
	}
	if n.Children != nil {
		cp.Children = make([]SemanticNode, len(n.Children))
		for i, child := range n.Children {
			cp.Children[i] = child.DeepCopy()
		}
	}
	return cp
}

// DeepCopy creates a deep copy of KnowledgeChunk.
func (c KnowledgeChunk) DeepCopy() KnowledgeChunk {
	cp := c
	if c.ParentChunkID != nil {
		val := *c.ParentChunkID
		cp.ParentChunkID = &val
	}
	if c.PreviousChunkID != nil {
		val := *c.PreviousChunkID
		cp.PreviousChunkID = &val
	}
	if c.NextChunkID != nil {
		val := *c.NextChunkID
		cp.NextChunkID = &val
	}
	if c.ChildChunkIDs != nil {
		cp.ChildChunkIDs = make([]string, len(c.ChildChunkIDs))
		copy(cp.ChildChunkIDs, c.ChildChunkIDs)
	}
	if c.PageNumbers != nil {
		cp.PageNumbers = make([]int, len(c.PageNumbers))
		copy(cp.PageNumbers, c.PageNumbers)
	}
	if c.Citations != nil {
		cp.Citations = make([]Citation, len(c.Citations))
		copy(cp.Citations, c.Citations)
	}
	return cp
}

// DeepCopy creates a deep copy of Assets.
func (a Assets) DeepCopy() Assets {
	cp := Assets{}
	if a.Tables != nil {
		cp.Tables = make([]Table, len(a.Tables))
		for i, tbl := range a.Tables {
			cp.Tables[i] = tbl
			if tbl.Headers != nil {
				cp.Tables[i].Headers = make([]string, len(tbl.Headers))
				copy(cp.Tables[i].Headers, tbl.Headers)
			}
		}
	}
	if a.Figures != nil {
		cp.Figures = make([]Figure, len(a.Figures))
		copy(cp.Figures, a.Figures)
	}
	if a.CodeBlocks != nil {
		cp.CodeBlocks = make([]CodeBlock, len(a.CodeBlocks))
		copy(cp.CodeBlocks, a.CodeBlocks)
	}
	if a.Equations != nil {
		cp.Equations = make([]Equation, len(a.Equations))
		copy(cp.Equations, a.Equations)
	}
	if a.Citations != nil {
		cp.Citations = make([]Citation, len(a.Citations))
		copy(cp.Citations, a.Citations)
	}
	if a.Warnings != nil {
		cp.Warnings = make([]ExtractionWarning, len(a.Warnings))
		copy(cp.Warnings, a.Warnings)
	}
	if a.Ordered != nil {
		cp.Ordered = make([]AssetReference, len(a.Ordered))
		copy(cp.Ordered, a.Ordered)
	}
	return cp
}

// DeepCopy creates a deep copy of Diagnostics.
func (d Diagnostics) DeepCopy() Diagnostics {
	cp := d
	if d.Warnings != nil {
		cp.Warnings = make([]string, len(d.Warnings))
		copy(cp.Warnings, d.Warnings)
	}
	if d.Errors != nil {
		cp.Errors = make([]string, len(d.Errors))
		copy(cp.Errors, d.Errors)
	}
	if d.SkippedPages != nil {
		cp.SkippedPages = make([]int, len(d.SkippedPages))
		copy(cp.SkippedPages, d.SkippedPages)
	}
	return cp
}
