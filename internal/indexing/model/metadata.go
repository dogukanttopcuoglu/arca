package model

import (
	"fmt"
)

// VectorMetadata aggregates provenance, schema, and versioning metadata attached to each stored vector point.
type VectorMetadata struct {
	WorkspaceID       string   `json:"workspace_id,omitempty"`
	KnowledgeSpaceID  string   `json:"knowledge_space_id,omitempty"`
	DocumentID        string   `json:"document_id"`
	ChunkID           string   `json:"chunk_id"`
	ChunkOrder        int      `json:"chunk_order"`
	SectionPath       string   `json:"section_path,omitempty"`
	PageNumbers       []int    `json:"page_numbers,omitempty"`
	ContentType       string   `json:"content_type,omitempty"`
	Citations         []string `json:"citations,omitempty"`
	ContentHash       string   `json:"content_hash,omitempty"`
	EmbeddingProvider string   `json:"embedding_provider,omitempty"`
	EmbeddingModel    string   `json:"embedding_model,omitempty"`
	EmbeddingVersion  string   `json:"embedding_version,omitempty"`
	ChunkSchemaVer    string   `json:"chunk_schema_version,omitempty"`
	IndexSignature    string   `json:"index_signature,omitempty"`
}

// Validate verifies structural invariants for VectorMetadata.
func (m VectorMetadata) Validate() error {
	if m.DocumentID == "" {
		return fmt.Errorf("document_id cannot be empty")
	}
	if m.ChunkID == "" {
		return fmt.Errorf("chunk_id cannot be empty")
	}
	return nil
}
