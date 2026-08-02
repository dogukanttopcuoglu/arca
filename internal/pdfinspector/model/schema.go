package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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
