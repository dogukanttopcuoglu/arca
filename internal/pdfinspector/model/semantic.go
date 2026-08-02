package model

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

// SemanticNode represents a heading/section node in the document hierarchy tree.
type SemanticNode struct {
	ID          string         `json:"id"`
	Heading     string         `json:"heading"`
	Level       int            `json:"level"`
	PageNumbers []int          `json:"pageNumbers"`
	Children    []SemanticNode `json:"children,omitempty"`
}

// SemanticTree represents the reconstructed hierarchical tree of document sections.
type SemanticTree struct {
	RootNodes []SemanticNode `json:"rootNodes"`
}
