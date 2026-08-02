package model

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

// Figure represents an extracted graphic, diagram, or picture.
type Figure struct {
	AssetMetadata
	Caption string `json:"caption,omitempty"`
	URI     string `json:"uri,omitempty"`
}

// GetMetadata implements Asset interface.
func (fig Figure) GetMetadata() AssetMetadata { return fig.AssetMetadata }

// CodeBlock represents an extracted code listing.
type CodeBlock struct {
	AssetMetadata
	Language string `json:"language,omitempty"`
	Content  string `json:"content"`
}

// GetMetadata implements Asset interface.
func (cb CodeBlock) GetMetadata() AssetMetadata { return cb.AssetMetadata }

// Equation represents an extracted LaTeX or mathematical formula.
type Equation struct {
	AssetMetadata
	LaTeX string `json:"latex"`
}

// GetMetadata implements Asset interface.
func (eq Equation) GetMetadata() AssetMetadata { return eq.AssetMetadata }

// Citation represents an inline or bibliographic document reference.
type Citation struct {
	AssetMetadata
	RawText      string       `json:"rawText"`
	CitationType CitationType `json:"citationType,omitempty"`
}

// GetMetadata implements Asset interface.
func (cit Citation) GetMetadata() AssetMetadata { return cit.AssetMetadata }

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
