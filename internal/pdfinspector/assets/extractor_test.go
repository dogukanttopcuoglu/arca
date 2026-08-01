package assets_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"arca/internal/pdfinspector/assets"
	"arca/internal/pdfinspector/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractor_Validation(t *testing.T) {
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

	t.Run("Validate vs ValidateComplete", func(t *testing.T) {
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

		require.NoError(t, meta.Validate())
		require.ErrorContains(t, meta.ValidateComplete(), "invalid page context")

		meta.PageNumber = 1
		require.NoError(t, meta.ValidateComplete())
	})
}

func TestExtractor_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	extractor := assets.NewExtractor()
	res, err := extractor.ExtractAssets(ctx, "# Test Document\n\nSome text.")
	assert.Nil(t, res)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestExtractor_OrderingIndependence(t *testing.T) {
	// Register extractors in reverse order: Citation, Figure, Table
	pipeline := assets.NewPipelineExtractor()
	pipeline.Register(assets.NewCitationExtractor())
	pipeline.Register(assets.NewFigureExtractor())
	pipeline.Register(assets.NewTableExtractor())

	markdown := `
# Sample Document

| Col A | Col B |
|-------|-------|
| Val 1 | Val 2 |

![Architecture](http://example.com/arch.png)

See reference [1] for details.
`

	res, err := pipeline.ExtractAssets(context.Background(), markdown)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify that Assets.Ordered is sorted strictly by StartOffset
	require.GreaterOrEqual(t, len(res.Ordered), 3)
	for i := 0; i < len(res.Ordered)-1; i++ {
		assert.LessOrEqual(t, res.Ordered[i].SourceLocation.StartOffset, res.Ordered[i+1].SourceLocation.StartOffset, "Assets.Ordered must be sorted strictly by StartOffset")
	}
}

func TestExtractor_Fixture_RickRubin(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "test", "fixtures", "rick-rubin.md")
	contentBytes, err := os.ReadFile(fixturePath)
	require.NoError(t, err, "rick-rubin.md fixture must exist at %s", fixturePath)

	markdown := string(contentBytes)
	extractor := assets.NewExtractor()

	res, err := extractor.ExtractAssets(context.Background(), markdown)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify catalog metadata, attributions, and HTML formatting extraction
	assert.Greater(t, res.Stats.CitationsFound, 0, "Citations should be extracted from rick-rubin.md")
	assert.Greater(t, res.Stats.DurationMs, int64(-1))
	assert.Equal(t, res.Stats.CitationsFound, len(res.Citations))

	// Verify attribution citation exists (e.g. ISBN or Library of Congress)
	foundCatalog := false
	for _, cit := range res.Citations {
		if cit.CitationType == model.CitationTypeAttribution {
			foundCatalog = true
			break
		}
	}
	assert.True(t, foundCatalog, "rick-rubin.md should yield attribution / catalog citations")
}

func TestExtractor_Fixture_AcademicPaper(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "test", "fixtures", "academic-paper.md")
	contentBytes, err := os.ReadFile(fixturePath)
	require.NoError(t, err)

	markdown := string(contentBytes)
	docContent := &model.DocumentContent{
		Markdown: markdown,
		PageMap: []model.PageMap{
			{PageNumber: 1, Markdown: markdown},
		},
	}
	chunks := []model.KnowledgeChunk{
		{
			ChunkID:       "chunk-1",
			PageNumbers:   []int{1},
			SourceOffsets: model.SourceOffset{StartChar: 0, EndChar: len(markdown)},
		},
	}

	extractor := assets.NewExtractor()
	res, err := extractor.ExtractAssetsWithContext(context.Background(), nil, docContent, chunks)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.GreaterOrEqual(t, len(res.Tables), 1, "Table should be extracted from academic-paper.md")
	assert.GreaterOrEqual(t, len(res.Equations), 1, "Math equations should be extracted from academic-paper.md")
	assert.GreaterOrEqual(t, len(res.Citations), 1, "Citations should be extracted from academic-paper.md")

	// Verify RelatedChunkIDs chunk linking
	if len(res.Tables) > 0 {
		assert.Contains(t, res.Tables[0].RelatedChunkIDs, "chunk-1")
	}
}

func TestExtractor_Fixture_TechnicalDocument(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "test", "fixtures", "technical-document.md")
	contentBytes, err := os.ReadFile(fixturePath)
	require.NoError(t, err)

	markdown := string(contentBytes)
	extractor := assets.NewExtractor()

	res, err := extractor.ExtractAssets(context.Background(), markdown)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.GreaterOrEqual(t, len(res.CodeBlocks), 2, "Code blocks (go, json) should be extracted")
	assert.GreaterOrEqual(t, len(res.Figures), 1, "Architecture diagram image should be extracted")

	foundGoCode := false
	for _, cb := range res.CodeBlocks {
		if cb.Language == "go" {
			foundGoCode = true
			break
		}
	}
	assert.True(t, foundGoCode, "Go code block should be extracted with language 'go'")
}

func TestExtractor_Fixture_MalformedHTML(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "test", "fixtures", "malformed-html.md")
	contentBytes, err := os.ReadFile(fixturePath)
	require.NoError(t, err)

	markdown := string(contentBytes)
	extractor := assets.NewExtractor()

	res, err := extractor.ExtractAssets(context.Background(), markdown)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify resilience: malformed HTML table generates warning without failing ingestion
	assert.GreaterOrEqual(t, res.Stats.WarningCount, 1, "Malformed HTML should emit extraction warnings")
	assert.GreaterOrEqual(t, len(res.Warnings), 1)
	assert.Equal(t, model.SeverityWarning, res.Warnings[0].Severity)
}

func TestExtractor_Fixture_MixedAssets(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "test", "fixtures", "mixed-assets.md")
	contentBytes, err := os.ReadFile(fixturePath)
	require.NoError(t, err)

	markdown := string(contentBytes)
	docContent := &model.DocumentContent{
		Markdown: markdown,
		PageMap: []model.PageMap{
			{PageNumber: 1, Markdown: markdown[:len(markdown)/2]},
			{PageNumber: 2, Markdown: markdown[len(markdown)/2:]},
		},
	}

	extractor := assets.NewExtractor()
	res, err := extractor.ExtractAssetsWithContext(context.Background(), nil, docContent, nil)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.GreaterOrEqual(t, len(res.Tables), 1)
	assert.GreaterOrEqual(t, len(res.Figures), 1)
	assert.GreaterOrEqual(t, len(res.CodeBlocks), 1)
	assert.GreaterOrEqual(t, len(res.Equations), 1)
	assert.GreaterOrEqual(t, len(res.Citations), 1)
	assert.Equal(t, len(res.Ordered), len(res.Tables)+len(res.Figures)+len(res.CodeBlocks)+len(res.Equations)+len(res.Citations))
}
