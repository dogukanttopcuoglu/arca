package model

// NodeType represents the structural type of a Knowledge Graph node.
type NodeType string

const (
	NodeTypeDocument NodeType = "document"
	NodeTypeSection  NodeType = "section"
	NodeTypeChunk    NodeType = "chunk"
	NodeTypeEntity   NodeType = "entity"
	NodeTypeConcept  NodeType = "concept"
	NodeTypeCitation NodeType = "citation"
)

// RelationType represents the direction/meaning of an Edge connection.
type RelationType string

const (
	RelationContains  RelationType = "contains"
	RelationReferences RelationType = "references"
	RelationCites     RelationType = "cites"
	RelationRelatedTo RelationType = "related_to"
)

// Node models a single vertex in the Knowledge Graph.
type Node struct {
	ID         string         `json:"id"`
	Type       NodeType       `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

// Edge models a directed connection between two graph vertices.
type Edge struct {
	From     string       `json:"from"`
	To       string       `json:"to"`
	Relation RelationType `json:"relation"`
}
