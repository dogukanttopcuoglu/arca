package model

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
