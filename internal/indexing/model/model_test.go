package model_test

import (
	"testing"
	"time"

	"arca/internal/indexing/model"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestVectorMetadataValidation(t *testing.T) {
	t.Run("valid vector metadata passes validation", func(t *testing.T) {
		meta := model.VectorMetadata{
			DocumentID:      "doc-123",
			ChunkID:         "chk-456",
			SectionPath:     "Architecture Overview/Key Subsystems",
			PageNumbers:     []int{1, 2},
			ContentHash:     "sha256:abc123def456",
			EmbeddingModel:  "text-embedding-3-large",
			EmbeddingVersion: "v1",
		}
		if err := meta.Validate(); err != nil {
			t.Errorf("expected valid metadata, got error: %v", err)
		}
	})

	t.Run("empty document ID returns validation error", func(t *testing.T) {
		meta := model.VectorMetadata{
			ChunkID:     "chk-456",
			SectionPath: "Overview",
		}
		if err := meta.Validate(); err == nil {
			t.Error("expected error for empty document ID, got nil")
		}
	})

	t.Run("empty chunk ID returns validation error", func(t *testing.T) {
		meta := model.VectorMetadata{
			DocumentID:  "doc-123",
			SectionPath: "Overview",
		}
		if err := meta.Validate(); err == nil {
			t.Error("expected error for empty chunk ID, got nil")
		}
	})
}

func TestCalculateIndexSignature(t *testing.T) {
	t.Run("generates deterministic hash signature for identical inputs", func(t *testing.T) {
		sig1 := model.CalculateIndexSignature("hash123", "OpenAI", "text-embedding-3-large", "v1", "1.0")
		sig2 := model.CalculateIndexSignature("hash123", "OpenAI", "text-embedding-3-large", "v1", "1.0")

		if sig1 == "" {
			t.Fatal("expected non-empty signature")
		}
		if sig1 != sig2 {
			t.Errorf("expected deterministic signatures, got %q vs %q", sig1, sig2)
		}
	})

	t.Run("signature changes when content hash or model changes", func(t *testing.T) {
		baseSig := model.CalculateIndexSignature("hash123", "OpenAI", "text-embedding-3-large", "v1", "1.0")
		diffContentSig := model.CalculateIndexSignature("hash999", "OpenAI", "text-embedding-3-large", "v1", "1.0")
		diffModelSig := model.CalculateIndexSignature("hash123", "OpenAI", "text-embedding-3-small", "v1", "1.0")

		if baseSig == diffContentSig {
			t.Error("expected signature to change when content hash changes")
		}
		if baseSig == diffModelSig {
			t.Error("expected signature to change when embedding model changes")
		}
	})
}

func TestMetadataFilterValidation(t *testing.T) {
	t.Run("valid metadata filter passes validation", func(t *testing.T) {
		now := time.Now()
		filter := model.MetadataFilter{
			WorkspaceID:      "ws-acme",
			KnowledgeSpaceID: "ks-design",
			DocumentIDs:      []string{"doc-1"},
			PageNumbers:      []int{1, 2},
			ContentTypes:     []string{pdfmodel.ContentTypeParagraph, pdfmodel.ContentTypeTable},
			IndexedAfter:     &now,
		}
		if err := filter.Validate(); err != nil {
			t.Errorf("expected valid filter, got error: %v", err)
		}
	})

	t.Run("invalid page number < 1 in filter returns error", func(t *testing.T) {
		filter := model.MetadataFilter{
			PageNumbers: []int{0},
		}
		if err := filter.Validate(); err == nil {
			t.Error("expected error for page number < 1, got nil")
		}
	})
}
