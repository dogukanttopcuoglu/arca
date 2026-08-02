package enrichment

import (
	"context"
	"strings"

	pdfmodel "arca/internal/pdfinspector/model"
)

// EntityExtractorPass implements EnricherPass for attaching entity mentions to chunks and performing minimal document-level grouping.
type EntityExtractorPass struct {
	extractor EntityExtractor
}

// NewEntityExtractorPass constructs an EntityExtractorPass instance.
func NewEntityExtractorPass(extractor EntityExtractor) *EntityExtractorPass {
	if extractor == nil {
		extractor = NewRuleBasedEntityExtractor()
	}
	return &EntityExtractorPass{
		extractor: extractor,
	}
}

func (p *EntityExtractorPass) Name() string { return "EntityExtractorPass" }
func (p *EntityExtractorPass) Requires() []Capability {
	return []Capability{CapabilityRawMetadata, CapabilityLanguage, CapabilitySemanticTree}
}
func (p *EntityExtractorPass) Provides() []Capability { return []Capability{CapabilityEntities} }

func (p *EntityExtractorPass) Execute(ctx context.Context, input *EnrichmentInput) error {
	if input == nil || len(input.Chunks) == 0 {
		return nil
	}

	lang := "en"
	if input.Metadata != nil && input.Metadata.Language != "" {
		lang = input.Metadata.Language
	}

	entityInput := EntityInput{
		Chunks:   input.Chunks,
		Language: lang,
		Labels:   []string{"person", "organization", "location", "product", "work of art"},
	}

	mentions, err := p.extractor.ExtractEntities(ctx, entityInput)
	if err != nil {
		return err
	}

	// 1. Attach mentions to individual KnowledgeChunks
	chunkMentionMap := make(map[string][]pdfmodel.EntityMention)
	for _, m := range mentions {
		chunkMentionMap[m.ChunkID] = append(chunkMentionMap[m.ChunkID], m)
	}

	for i := range input.Chunks {
		if ms, ok := chunkMentionMap[input.Chunks[i].ChunkID]; ok {
			input.Chunks[i].Entities = ms
		}
	}

	// 2. Perform minimal in-memory grouping by name & type to populate DocumentMetadata.Entities
	if input.Metadata != nil {
		entityGroupMap := make(map[string]*pdfmodel.Entity)
		var entityOrder []string

		for _, m := range mentions {
			key := string(m.Type) + ":" + strings.ToLower(strings.TrimSpace(m.Text))
			if ent, exists := entityGroupMap[key]; exists {
				ent.Mentions = append(ent.Mentions, m)
				ent.Score += 0.1
				if ent.Score > 1.0 {
					ent.Score = 1.0
				}
			} else {
				entityOrder = append(entityOrder, key)
				entityGroupMap[key] = &pdfmodel.Entity{
					ID:       key,
					Name:     m.Text,
					Type:     m.Type,
					Mentions: []pdfmodel.EntityMention{m},
					Score:    0.80,
				}
			}
		}

		var docEntities []pdfmodel.Entity
		for _, key := range entityOrder {
			if ent, ok := entityGroupMap[key]; ok {
				docEntities = append(docEntities, *ent)
			}
		}
		input.Metadata.Entities = docEntities
	}

	return nil
}
