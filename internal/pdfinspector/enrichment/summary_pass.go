package enrichment

import (
	"context"

	pdfmodel "arca/internal/pdfinspector/model"
)

// SummaryPass implements EnricherPass for executive and section-level summary extraction.
type SummaryPass struct {
	extractor SummaryExtractor
}

// NewSummaryPass constructs a SummaryPass instance.
func NewSummaryPass(extractor SummaryExtractor) *SummaryPass {
	if extractor == nil {
		extractor = NewRuleBasedSummaryExtractor()
	}
	return &SummaryPass{
		extractor: extractor,
	}
}

func (p *SummaryPass) Name() string { return "SummaryPass" }
func (p *SummaryPass) Requires() []Capability {
	return []Capability{CapabilityKeywords, CapabilityEntities, CapabilityConcepts, CapabilityRelations}
}
func (p *SummaryPass) Provides() []Capability { return []Capability{CapabilitySummary} }

func (p *SummaryPass) Execute(ctx context.Context, input *EnrichmentInput) error {
	if input == nil {
		return nil
	}

	var keywords []pdfmodel.Keyword
	if input.Metadata != nil {
		keywords = input.Metadata.Keywords
	}

	var entities []pdfmodel.Entity
	if input.Metadata != nil {
		entities = input.Metadata.Entities
	}

	var concepts []pdfmodel.Concept
	if input.Metadata != nil {
		concepts = input.Metadata.Concepts
	}

	var relations []pdfmodel.Relation
	if input.Metadata != nil {
		relations = input.Metadata.Relations
	}

	sumInput := SummaryInput{
		Chunks:    input.Chunks,
		Keywords:  keywords,
		Entities:  entities,
		Concepts:  concepts,
		Relations: relations,
	}

	res, err := p.extractor.ExtractSummaries(ctx, sumInput)
	if err != nil {
		return err
	}

	if input.Metadata != nil && res.DocumentSummary != nil {
		input.Metadata.Summary = res.DocumentSummary
	}

	for i := range input.Chunks {
		if sum, ok := res.ChunkSummaries[input.Chunks[i].ChunkID]; ok {
			input.Chunks[i].Summary = sum
		}
	}

	return nil
}
