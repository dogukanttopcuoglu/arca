package model_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"arca/internal/pdfinspector/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPDFInspectionResult_Constructor(t *testing.T) {
	res := model.NewPDFInspectionResult()

	assert.Equal(t, model.SchemaVersionV1, res.SchemaVersion)
	assert.NotNil(t, res.Document.Fonts)
	assert.NotNil(t, res.SemanticTree.RootNodes)
	assert.NotNil(t, res.Content.PageMap)
	assert.NotNil(t, res.Chunks)
	assert.NotNil(t, res.Assets.Tables)
	assert.NotNil(t, res.Assets.Figures)
	assert.NotNil(t, res.Assets.CodeBlocks)
	assert.NotNil(t, res.Assets.Equations)
	assert.NotNil(t, res.Assets.Citations)
	assert.NotNil(t, res.Diagnostics.Warnings)
	assert.NotNil(t, res.Diagnostics.Errors)
	assert.NotNil(t, res.Diagnostics.SkippedPages)
	assert.Equal(t, model.StatusSuccess, res.Diagnostics.Status)

	// Serialize and verify slices render as empty JSON arrays [], not null
	data, err := res.ToJSON()
	require.NoError(t, err)
	jsonStr := string(data)

	assert.Contains(t, jsonStr, `"schemaVersion":"1.0.0"`)
	assert.Contains(t, jsonStr, `"chunks":[]`)
	assert.Contains(t, jsonStr, `"pageMap":[]`)
	assert.Contains(t, jsonStr, `"warnings":[]`)
}

func TestPDFInspectionResult_SerializationRoundTrip(t *testing.T) {
	parentID := "chunk-parent-1"
	now := time.Now().Truncate(time.Second).UTC()

	original := model.NewPDFInspectionResult()
	original.Document = model.DocumentMetadata{
		Title:            "Test Document",
		Author:           "Test Author",
		Creator:          "Test Creator",
		Producer:         "Test Producer",
		CreationDate:     now,
		ModificationDate: now,
		PageCount:        5,
		PageDimensions:   "Letter",
		Fonts:            []string{"Helvetica", "Courier"},
		Encrypted:        false,
		Searchable:       true,
	}
	original.SemanticTree = model.SemanticTree{
		RootNodes: []model.SemanticNode{
			{
				ID:          "sec-1",
				Heading:     "Introduction",
				Level:       1,
				PageNumbers: []int{1},
				Children: []model.SemanticNode{
					{
						ID:          "sec-1-1",
						Heading:     "Background",
						Level:       2,
						PageNumbers: []int{1, 2},
					},
				},
			},
		},
	}
	original.Content = model.DocumentContent{
		Markdown: "# Introduction\n\nSome background info.",
		PageMap: []model.PageMap{
			{PageNumber: 1, Markdown: "# Introduction"},
			{PageNumber: 2, Markdown: "Some background info."},
		},
	}
	original.Chunks = []model.KnowledgeChunk{
		{
			ChunkID:         "chunk-parent-1",
			ParentChunkID:   nil,
			ChildChunkIDs:   []string{"chunk-child-1"},
			DocumentID:      "doc-123",
			SectionPath:     "Introduction",
			HeadingLevel:    1,
			PageNumbers:     []int{1},
			ContentMarkdown: "# Introduction",
			TokenEstimate:   10,
			CharacterCount:  14,
			SourceOffsets: model.SourceOffset{
				StartChar: 0,
				EndChar:   14,
			},
			ContentType: model.ContentTypeParagraph,
		},
		{
			ChunkID:         "chunk-child-1",
			ParentChunkID:   &parentID,
			ChildChunkIDs:   []string{},
			DocumentID:      "doc-123",
			SectionPath:     "Introduction > Background",
			HeadingLevel:    2,
			PageNumbers:     []int{1, 2},
			ContentMarkdown: "Some background info.",
			TokenEstimate:   20,
			CharacterCount:  21,
			SourceOffsets: model.SourceOffset{
				StartChar: 16,
				EndChar:   37,
			},
			ContentType: model.ContentTypeParagraph,
		},
	}
	original.Assets = model.Assets{
		Tables: []model.Table{
			{
				AssetMetadata: model.AssetMetadata{
					ID:         "tbl-1",
					AssetType:  model.AssetTypeTable,
					PageNumber: 1,
				},
				Caption: "Table 1",
				Content: "| A | B |\n|---|---|\n| 1 | 2 |",
				Headers: []string{"A", "B"},
			},
		},
		Figures: []model.Figure{
			{
				AssetMetadata: model.AssetMetadata{
					ID:         "fig-1",
					AssetType:  model.AssetTypeFigure,
					PageNumber: 2,
				},
				Caption: "Figure 1",
				URI:     "http://example.com/fig1.png",
			},
		},
		CodeBlocks: []model.CodeBlock{
			{
				AssetMetadata: model.AssetMetadata{
					ID:         "code-1",
					AssetType:  model.AssetTypeCodeBlock,
					PageNumber: 2,
				},
				Language: "go",
				Content:  "fmt.Println(\"hello\")",
			},
		},
		Equations: []model.Equation{
			{
				AssetMetadata: model.AssetMetadata{
					ID:         "eq-1",
					AssetType:  model.AssetTypeEquation,
					PageNumber: 3,
				},
				LaTeX: "E = mc^2",
			},
		},
		Citations: []model.Citation{
			{
				AssetMetadata: model.AssetMetadata{
					ID:         "cit-1",
					AssetType:  model.AssetTypeCitation,
					PageNumber: 3,
				},
				RawText:      "Smith et al. 2026",
				CitationType: model.CitationTypeInline,
			},
		},
	}
	original.Diagnostics = model.Diagnostics{
		Status:           model.StatusSuccess,
		ExtractionEngine: "firecrawl",
		ExtractionVer:    "1.0.0",
		ProcessingTimeMs: 150,
		Warnings:         []string{"Low DPI detected on page 2"},
		Errors:           []string{},
		SkippedPages:     []int{},
		RetryCount:       1,
	}

	require.NoError(t, original.Validate())

	data, err := original.ToJSONIndent("", "  ")
	require.NoError(t, err)

	deserialized, err := model.PDFInspectionResultFromJSON(data)
	require.NoError(t, err)

	require.NoError(t, deserialized.Validate())
	assert.Equal(t, original.SchemaVersion, deserialized.SchemaVersion)
	assert.Equal(t, original.Document.Title, deserialized.Document.Title)
	assert.Equal(t, original.Document.PageCount, deserialized.Document.PageCount)
	assert.Equal(t, len(original.Chunks), len(deserialized.Chunks))
	assert.Equal(t, original.Chunks[0].ChunkID, deserialized.Chunks[0].ChunkID)
	assert.Nil(t, deserialized.Chunks[0].ParentChunkID)
	assert.NotNil(t, deserialized.Chunks[1].ParentChunkID)
	assert.Equal(t, "chunk-parent-1", *deserialized.Chunks[1].ParentChunkID)
}

func TestKnowledgeChunk_ParentChunkIDNullSerialization(t *testing.T) {
	chunk := model.KnowledgeChunk{
		ChunkID:         "chunk-1",
		ParentChunkID:   nil,
		ChildChunkIDs:   []string{},
		DocumentID:      "doc-1",
		SectionPath:     "Section 1",
		HeadingLevel:    1,
		PageNumbers:     []int{1},
		ContentMarkdown: "Content",
		TokenEstimate:   5,
		CharacterCount:  7,
		SourceOffsets:   model.SourceOffset{StartChar: 0, EndChar: 7},
		ContentType:     model.ContentTypeParagraph,
	}

	data, err := json.Marshal(chunk)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"parent_chunk_id":null`)

	parentID := "chunk-0"
	chunk.ParentChunkID = &parentID
	dataWithParent, err := json.Marshal(chunk)
	require.NoError(t, err)
	assert.Contains(t, string(dataWithParent), `"parent_chunk_id":"chunk-0"`)
}

func TestPDFInspectionResult_ValidationRules(t *testing.T) {
	t.Run("Valid default model", func(t *testing.T) {
		res := model.NewPDFInspectionResult()
		assert.NoError(t, res.Validate())
	})

	t.Run("Nil model", func(t *testing.T) {
		var res *model.PDFInspectionResult
		assert.ErrorContains(t, res.Validate(), "cannot be nil")
	})

	t.Run("Invalid SchemaVersion", func(t *testing.T) {
		res := model.NewPDFInspectionResult()
		res.SchemaVersion = "2.0.0"
		assert.ErrorContains(t, res.Validate(), "unsupported schemaVersion")
	})

	t.Run("Negative Document PageCount", func(t *testing.T) {
		res := model.NewPDFInspectionResult()
		res.Document.PageCount = -1
		assert.ErrorContains(t, res.Validate(), "invalid pageCount")
	})

	t.Run("Invalid Diagnostics status", func(t *testing.T) {
		res := model.NewPDFInspectionResult()
		res.Diagnostics.Status = "UNKNOWN"
		assert.ErrorContains(t, res.Validate(), "invalid diagnostics status")
	})

	t.Run("Invalid KnowledgeChunk ContentType", func(t *testing.T) {
		res := model.NewPDFInspectionResult()
		res.Chunks = []model.KnowledgeChunk{
			{
				ChunkID:        "c1",
				ContentType:    "unknown_type",
				HeadingLevel:   1,
				TokenEstimate:  10,
				CharacterCount: 10,
				PageNumbers:    []int{1},
				SourceOffsets:  model.SourceOffset{StartChar: 0, EndChar: 10},
			},
		}
		assert.ErrorContains(t, res.Validate(), "unsupported content_type")
	})

	t.Run("Invalid SourceOffset bounds", func(t *testing.T) {
		so := model.SourceOffset{StartChar: 10, EndChar: 5}
		assert.ErrorContains(t, so.Validate(), "start_char 10 > end_char 5")
	})

	t.Run("Invalid PageNumber < 1", func(t *testing.T) {
		pm := model.PageMap{PageNumber: 0, Markdown: "test"}
		assert.ErrorContains(t, pm.Validate(), "invalid pageNumber in PageMap: 0")
	})

	t.Run("Missing parent_chunk_id reference", func(t *testing.T) {
		res := model.NewPDFInspectionResult()
		missingParent := "non-existent-chunk"
		res.Chunks = []model.KnowledgeChunk{
			{
				ChunkID:        "c1",
				ParentChunkID:  &missingParent,
				ContentType:    model.ContentTypeParagraph,
				HeadingLevel:   1,
				TokenEstimate:  10,
				CharacterCount: 10,
				PageNumbers:    []int{1},
				SourceOffsets:  model.SourceOffset{StartChar: 0, EndChar: 10},
			},
		}
		assert.ErrorContains(t, res.Validate(), "references missing parent_chunk_id non-existent-chunk")
	})

	t.Run("Missing child_chunk_id reference", func(t *testing.T) {
		res := model.NewPDFInspectionResult()
		res.Chunks = []model.KnowledgeChunk{
			{
				ChunkID:        "c1",
				ChildChunkIDs:  []string{"non-existent-child"},
				ContentType:    model.ContentTypeParagraph,
				HeadingLevel:   1,
				TokenEstimate:  10,
				CharacterCount: 10,
				PageNumbers:    []int{1},
				SourceOffsets:  model.SourceOffset{StartChar: 0, EndChar: 10},
			},
		}
		assert.ErrorContains(t, res.Validate(), "references missing child_chunk_id non-existent-child")
	})
}

func TestPDFInspectionResult_JSONSchemaCompliance(t *testing.T) {
	// Find schema path relative to project root
	schemaPath := filepath.Join("..", "..", "..", "docs", "schemas", "pdf-inspection-result-v1.json")
	schemaBytes, err := os.ReadFile(schemaPath)
	require.NoError(t, err, "schema file should exist at %s", schemaPath)

	var schemaMap map[string]interface{}
	require.NoError(t, json.Unmarshal(schemaBytes, &schemaMap))

	res := model.NewPDFInspectionResult()
	res.Document.Title = "Schema Test Doc"
	res.Document.PageCount = 2
	res.Document.Searchable = true
	res.Document.Encrypted = false

	data, err := res.ToJSON()
	require.NoError(t, err)

	var outputMap map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &outputMap))

	// Structural verification against schema requirements
	requiredFields := []string{"schemaVersion", "document", "semanticTree", "content", "chunks", "assets", "diagnostics"}
	for _, field := range requiredFields {
		assert.Contains(t, outputMap, field, "output JSON must contain required field: %s", field)
	}

	assert.Equal(t, "1.0.0", outputMap["schemaVersion"])
}

func TestPDFInspectionResult_DeepCopy(t *testing.T) {
	parentID := "chunk-p"
	orig := model.NewPDFInspectionResult()
	orig.Document.Fonts = []string{"Arial"}
	orig.Content.PageMap = []model.PageMap{{PageNumber: 1, Markdown: "Original"}}
	orig.Chunks = []model.KnowledgeChunk{
		{
			ChunkID:       "chunk-p",
			ChildChunkIDs: []string{"chunk-c"},
			PageNumbers:   []int{1},
		},
		{
			ChunkID:       "chunk-c",
			ParentChunkID: &parentID,
			ChildChunkIDs: []string{},
			PageNumbers:   []int{1},
		},
	}
	orig.Diagnostics.Warnings = []string{"Warning 1"}

	cloned := orig.DeepCopy()

	// Mutate cloned fields
	cloned.Document.Fonts[0] = "Times"
	cloned.Content.PageMap[0].Markdown = "Mutated"
	cloned.Chunks[0].ChildChunkIDs[0] = "chunk-mutated"
	cloned.Diagnostics.Warnings[0] = "Mutated Warning"

	// Verify original is untouched
	assert.Equal(t, "Arial", orig.Document.Fonts[0])
	assert.Equal(t, "Original", orig.Content.PageMap[0].Markdown)
	assert.Equal(t, "chunk-c", orig.Chunks[0].ChildChunkIDs[0])
	assert.Equal(t, "Warning 1", orig.Diagnostics.Warnings[0])

	// Also verify Clone alias
	clonedAlias := orig.Clone()
	assert.Equal(t, "Arial", clonedAlias.Document.Fonts[0])
}

func TestAssetMetadata_Validation(t *testing.T) {
	t.Run("AssetType and CitationType validity", func(t *testing.T) {
		assert.True(t, model.AssetTypeTable.IsValid())
		assert.True(t, model.AssetTypeFigure.IsValid())
		assert.True(t, model.AssetTypeCodeBlock.IsValid())
		assert.True(t, model.AssetTypeEquation.IsValid())
		assert.True(t, model.AssetTypeCitation.IsValid())
		assert.False(t, model.AssetType("invalid").IsValid())

		assert.True(t, model.CitationTypeInline.IsValid())
		assert.True(t, model.CitationTypeFootnote.IsValid())
		assert.True(t, model.CitationTypeBibliography.IsValid())
		assert.True(t, model.CitationTypeAttribution.IsValid())
		assert.False(t, model.CitationType("unknown").IsValid())
	})

	t.Run("Basic Validate vs ValidateComplete", func(t *testing.T) {
		meta := model.AssetMetadata{
			ID:        "tbl-1",
			AssetType: model.AssetTypeTable,
			SourceLocation: model.SourceLocation{
				StartOffset: 0,
				EndOffset:   10,
				StartLine:   1,
				EndLine:     2,
			},
		}

		// Basic validation succeeds before page resolution
		assert.NoError(t, meta.Validate())
		// Complete validation fails because PageNumber is 0
		assert.ErrorContains(t, meta.ValidateComplete(), "invalid page context")

		meta.PageNumber = 1
		assert.NoError(t, meta.ValidateComplete())
	})

	t.Run("Asset interface GetMetadata polymorphism", func(t *testing.T) {
		tbl := model.Table{
			AssetMetadata: model.AssetMetadata{
				ID:        "tbl-99",
				AssetType: model.AssetTypeTable,
			},
			Caption: "Sample Table",
		}

		var asset model.Asset = tbl
		assert.Equal(t, "tbl-99", asset.GetMetadata().ID)
		assert.Equal(t, model.AssetTypeTable, asset.GetMetadata().AssetType)
	})
}

