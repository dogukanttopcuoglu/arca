package model

import (
	"crypto/sha256"
	"fmt"
)

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
	RelationContains   RelationType = "contains"
	RelationReferences RelationType = "references"
	RelationCites      RelationType = "cites"
	RelationRelatedTo  RelationType = "related_to"
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

// CalculatePointID generates a deterministic, stable point ID for a graph
// node: SHA256(nodeID) truncated to 128 bits and formatted as a UUID
// (RFC 4122 variant bits), so the ID is valid for Qdrant PointId (UUID)
// while remaining stable across re-indexing. It is the single UUID-format
// helper; the chunk-point counterpart in internal/indexing/store delegates
// to it.
func CalculatePointID(nodeID string) string {
	h := sha256.Sum256([]byte(nodeID))
	h[6] = (h[6] & 0x0f) | 0x50
	h[8] = (h[8] & 0x3f) | 0x80
	hexstr := fmt.Sprintf("%x", h[:16])
	return hexstr[0:8] + "-" + hexstr[8:12] + "-" + hexstr[12:16] + "-" + hexstr[16:20] + "-" + hexstr[20:32]
}
