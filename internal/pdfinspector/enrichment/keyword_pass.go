package enrichment

import (
	"context"

	pdfmodel "arca/internal/pdfinspector/model"
)

// KeywordExtractorPass implements EnricherPass for attaching extracted keywords to document and chunk levels.
type KeywordExtractorPass struct {
	extractor KeywordExtractor
}

// NewKeywordExtractorPass constructs a KeywordExtractorPass instance.
func NewKeywordExtractorPass(extractor KeywordExtractor) *KeywordExtractorPass {
	if extractor == nil {
		extractor = NewRuleBasedKeywordExtractor()
	}
	return &KeywordExtractorPass{
		extractor: extractor,
	}
}

func (p *KeywordExtractorPass) Name() string { return "KeywordExtractorPass" }
func (p *KeywordExtractorPass) Requires() []Capability {
	return []Capability{CapabilityRawMetadata, CapabilityLanguage, CapabilityEntities}
}
func (p *KeywordExtractorPass) Provides() []Capability { return []Capability{CapabilityKeywords} }

func (p *KeywordExtractorPass) Execute(ctx context.Context, input *EnrichmentInput) error {
	if input == nil || len(input.Chunks) == 0 {
		return nil
	}

	lang := "en"
	if input.Metadata != nil && input.Metadata.Language != "" {
		lang = input.Metadata.Language
	}

	// Collect document-level entities for entity-aware keyword filtering
	var entities []pdfmodel.Entity
	if input.Metadata != nil {
		entities = input.Metadata.Entities
	}

	keywords, err := p.extractor.Extract(ctx, input.Chunks, lang, entities)
	if err != nil {
		return err
	}

	// 1. Attach document-level keywords
	if input.Metadata != nil {
		input.Metadata.Keywords = keywords
	}

	// 2. Attach chunk-level keywords for RAG retrieval
	chunkKeywordMap := make(map[string][]pdfmodel.Keyword)
	for _, kw := range keywords {
		for _, chunkID := range kw.ChunkIDs {
			chunkKeywordMap[chunkID] = append(chunkKeywordMap[chunkID], kw)
		}
	}

	for i := range input.Chunks {
		if kws, ok := chunkKeywordMap[input.Chunks[i].ChunkID]; ok {
			input.Chunks[i].Keywords = kws
		}
	}

	return nil
}
