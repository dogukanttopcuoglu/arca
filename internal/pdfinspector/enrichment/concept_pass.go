package enrichment

import (
	"context"
	"strings"

	pdfmodel "arca/internal/pdfinspector/model"
)

// ConceptExtractorPass implements EnricherPass for abstract topic and thematic concept discovery.
type ConceptExtractorPass struct {
	extractor ConceptExtractor
}

// NewConceptExtractorPass constructs a ConceptExtractorPass instance.
func NewConceptExtractorPass(extractor ConceptExtractor) *ConceptExtractorPass {
	if extractor == nil {
		extractor = NewRuleBasedConceptExtractor()
	}
	return &ConceptExtractorPass{
		extractor: extractor,
	}
}

func (p *ConceptExtractorPass) Name() string { return "ConceptExtractorPass" }
func (p *ConceptExtractorPass) Requires() []Capability {
	return []Capability{CapabilityRawMetadata, CapabilityLanguage, CapabilitySemanticTree, CapabilityKeywords, CapabilityEntities}
}
func (p *ConceptExtractorPass) Provides() []Capability { return []Capability{CapabilityConcepts} }

func (p *ConceptExtractorPass) Execute(ctx context.Context, input *EnrichmentInput) ([]string, error) {
	if input == nil {
		return nil, nil
	}

	lang := "en"
	if input.Metadata != nil && input.Metadata.Language != "" {
		lang = input.Metadata.Language
	}

	var keywords []pdfmodel.Keyword
	if input.Metadata != nil {
		keywords = input.Metadata.Keywords
	}

	var entities []pdfmodel.Entity
	if input.Metadata != nil {
		entities = input.Metadata.Entities
	}

	conceptInput := ConceptInput{
		Tree:     input.Tree,
		Chunks:   input.Chunks,
		Keywords: keywords,
		Entities: entities,
		Language: lang,
	}

	concepts, err := p.extractor.ExtractConcepts(ctx, conceptInput)
	if err != nil {
		return nil, err
	}

	if input.Metadata != nil {
		input.Metadata.Concepts = concepts
	}

	// Attach relevant concepts to individual KnowledgeChunks
	for i := range input.Chunks {
		chunkPathLower := strings.ToLower(strings.TrimSpace(input.Chunks[i].SectionPath))
		chunkTextLower := strings.ToLower(strings.TrimSpace(input.Chunks[i].ContentMarkdown))

		var chunkConcepts []pdfmodel.Concept
		for _, c := range concepts {
			conceptNameLower := strings.ToLower(strings.TrimSpace(c.Name))
			if conceptNameLower != "" && ((chunkPathLower != "" && strings.Contains(chunkPathLower, conceptNameLower)) ||
				strings.Contains(chunkTextLower, conceptNameLower)) {
				chunkConcepts = append(chunkConcepts, c)
			}
		}
		input.Chunks[i].Concepts = chunkConcepts
	}

	return nil, nil
}
