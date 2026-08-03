package enrichment

import (
	"context"

	pdfmodel "arca/internal/pdfinspector/model"
)

// RelationExtractorPass implements EnricherPass for directed Knowledge Graph relationship extraction.
type RelationExtractorPass struct {
	extractor RelationExtractor
}

// NewRelationExtractorPass constructs a RelationExtractorPass instance.
func NewRelationExtractorPass(extractor RelationExtractor) *RelationExtractorPass {
	if extractor == nil {
		extractor = NewRuleBasedRelationExtractor()
	}
	return &RelationExtractorPass{
		extractor: extractor,
	}
}

func (p *RelationExtractorPass) Name() string { return "RelationExtractorPass" }
func (p *RelationExtractorPass) Requires() []Capability {
	return []Capability{CapabilityEntities, CapabilityConcepts}
}
func (p *RelationExtractorPass) Provides() []Capability { return []Capability{CapabilityRelations} }

func (p *RelationExtractorPass) Execute(ctx context.Context, input *EnrichmentInput) ([]string, error) {
	if input == nil {
		return nil, nil
	}

	var entities []pdfmodel.Entity
	if input.Metadata != nil {
		entities = input.Metadata.Entities
	}

	var concepts []pdfmodel.Concept
	if input.Metadata != nil {
		concepts = input.Metadata.Concepts
	}

	relInput := RelationInput{
		Chunks:   input.Chunks,
		Entities: entities,
		Concepts: concepts,
	}

	relations, err := p.extractor.ExtractRelations(ctx, relInput)
	if err != nil {
		return nil, err
	}

	// 1. Populate canonical deduplicated relation catalog on DocumentMetadata
	if input.Metadata != nil {
		seen := make(map[string]bool)
		var catalog []pdfmodel.Relation
		for _, rel := range relations {
			if !seen[rel.ID] {
				seen[rel.ID] = true
				catalog = append(catalog, rel)
			}
		}
		input.Metadata.Relations = catalog
	}

	// 2. Attach chunk-specific relations to KnowledgeChunks
	chunkRelMap := make(map[string][]pdfmodel.Relation)
	for _, rel := range relations {
		if rel.ChunkID != "" {
			chunkRelMap[rel.ChunkID] = append(chunkRelMap[rel.ChunkID], rel)
		}
	}

	for i := range input.Chunks {
		if rels, ok := chunkRelMap[input.Chunks[i].ChunkID]; ok {
			input.Chunks[i].Relations = rels
		}
	}

	return nil, nil
}
