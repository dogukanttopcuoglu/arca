package model

import "fmt"

// Validatable describes models capable of self-validation for structural correctness and domain invariants.
type Validatable interface {
	Validate() error
}

// Validate verifies structural correctness for DocumentMetadata.
func (m DocumentMetadata) Validate() error {
	if m.PageCount < 0 {
		return fmt.Errorf("invalid pageCount: %d (must be >= 0)", m.PageCount)
	}
	return nil
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

// Validate verifies structural invariants for SemanticTree.
func (t SemanticTree) Validate() error {
	for _, root := range t.RootNodes {
		if err := root.Validate(); err != nil {
			return fmt.Errorf("invalid root node: %w", err)
		}
	}
	return nil
}

// Validate verifies page map structural invariants.
func (pm PageMap) Validate() error {
	if pm.PageNumber < 1 {
		return fmt.Errorf("invalid pageNumber in PageMap: %d (must be >= 1)", pm.PageNumber)
	}
	return nil
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

// Validate verifies Table invariants.
func (tbl Table) Validate() error {
	return tbl.AssetMetadata.Validate()
}

// Validate verifies Figure invariants.
func (fig Figure) Validate() error {
	return fig.AssetMetadata.Validate()
}

// Validate verifies CodeBlock invariants.
func (cb CodeBlock) Validate() error {
	return cb.AssetMetadata.Validate()
}

// Validate verifies Equation invariants.
func (eq Equation) Validate() error {
	return eq.AssetMetadata.Validate()
}

// Validate verifies Citation invariants.
func (cit Citation) Validate() error {
	return cit.AssetMetadata.Validate()
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
