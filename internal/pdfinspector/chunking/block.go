package chunking

import "arca/internal/pdfinspector/model"

// BlockKind identifies the semantic category of an intermediate block.
type BlockKind string

const (
	KindParagraph BlockKind = "paragraph"
	KindTable     BlockKind = "table"
	KindCode      BlockKind = "code"
	KindList      BlockKind = "list"
	KindEquation  BlockKind = "equation"
	KindFigure    BlockKind = "figure"
)

// SemanticBlock represents a single atomic or composite document block prior to chunking.
type SemanticBlock struct {
	Kind             BlockKind
	HeadingLevel     int
	SectionPath      string
	Markdown         string
	PageNumbers      []int
	SourceOffsets    model.SourceOffset
	Citations        []model.Citation
	SemanticCategory model.SemanticCategory
}
