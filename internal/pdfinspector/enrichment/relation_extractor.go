package enrichment

import (
	"context"
	"strings"

	pdfmodel "arca/internal/pdfinspector/model"
)

// RelationInput holds targeted inputs for relation extraction.
type RelationInput struct {
	Chunks   []pdfmodel.KnowledgeChunk
	Entities []pdfmodel.Entity
	Concepts []pdfmodel.Concept
}

// RelationExtractor defines the strategy seam for extracting directed SPO relationships.
type RelationExtractor interface {
	ExtractRelations(ctx context.Context, input RelationInput) ([]pdfmodel.Relation, error)
}

// RuleBasedRelationExtractor implements RelationExtractor using pattern co-occurrence and predicate heuristics.
type RuleBasedRelationExtractor struct{}

// NewRuleBasedRelationExtractor constructs a RuleBasedRelationExtractor instance.
func NewRuleBasedRelationExtractor() *RuleBasedRelationExtractor {
	return &RuleBasedRelationExtractor{}
}

// ExtractRelations discovers directed relationships between entities and concepts.
func (e *RuleBasedRelationExtractor) ExtractRelations(ctx context.Context, input RelationInput) ([]pdfmodel.Relation, error) {
	if len(input.Chunks) == 0 && len(input.Entities) == 0 {
		return []pdfmodel.Relation{}, nil
	}

	var relations []pdfmodel.Relation
	seen := make(map[string]bool)

	// Map entity/concept names to canonical IDs
	entityMap := make(map[string]pdfmodel.Entity)
	for _, ent := range input.Entities {
		entityMap[strings.ToLower(ent.Name)] = ent
	}

	conceptMap := make(map[string]pdfmodel.Concept)
	for _, c := range input.Concepts {
		conceptMap[strings.ToLower(c.Name)] = c
	}

	for _, ch := range input.Chunks {
		text := ch.ContentMarkdown
		textLower := strings.ToLower(text)

		// 1. Entity ↔ Entity Relations (e.g. Person founded Organization)
		var personEntities []pdfmodel.Entity
		var orgEntities []pdfmodel.Entity
		var locEntities []pdfmodel.Entity

		for _, m := range ch.Entities {
			mLower := strings.ToLower(m.Text)
			if ent, ok := entityMap[mLower]; ok {
				switch ent.Type {
				case pdfmodel.EntityTypePerson:
					personEntities = append(personEntities, ent)
				case pdfmodel.EntityTypeOrganization:
					orgEntities = append(orgEntities, ent)
				case pdfmodel.EntityTypeLocation:
					locEntities = append(locEntities, ent)
				}
			}
		}

		if (strings.Contains(textLower, "founded") || strings.Contains(textLower, "created") || strings.Contains(textLower, "started")) &&
			len(personEntities) > 0 && len(orgEntities) > 0 {
			for _, p := range personEntities {
				for _, o := range orgEntities {
					relID := "rel:" + o.ID + ":" + string(pdfmodel.RelationTypeFoundedBy) + ":" + p.ID
					if !seen[relID] {
						seen[relID] = true
						relations = append(relations, pdfmodel.Relation{
							ID:         relID,
							SubjectID:  o.ID,
							Predicate:  pdfmodel.RelationTypeFoundedBy,
							ObjectID:   p.ID,
							Confidence: 0.90,
							ChunkID:    ch.ChunkID,
							Source:     pdfmodel.RelationSourceRuleBased,
						})
					}
				}
			}
		}

		if (strings.Contains(textLower, "in ") || strings.Contains(textLower, "at ")) &&
			len(orgEntities) > 0 && len(locEntities) > 0 {
			for _, o := range orgEntities {
				for _, l := range locEntities {
					relID := "rel:" + o.ID + ":" + string(pdfmodel.RelationTypeLocatedIn) + ":" + l.ID
					if !seen[relID] {
						seen[relID] = true
						relations = append(relations, pdfmodel.Relation{
							ID:         relID,
							SubjectID:  o.ID,
							Predicate:  pdfmodel.RelationTypeLocatedIn,
							ObjectID:   l.ID,
							Confidence: 0.85,
							ChunkID:    ch.ChunkID,
							Source:     pdfmodel.RelationSourceRuleBased,
						})
					}
				}
			}
		}

		// 2. Entity ↔ Concept Relations (e.g. Entity relates to Concept)
		for _, m := range ch.Entities {
			mLower := strings.ToLower(m.Text)
			if ent, ok := entityMap[mLower]; ok {
				for _, c := range input.Concepts {
					cLower := strings.ToLower(c.Name)
					if strings.Contains(textLower, cLower) {
						relID := "rel:" + ent.ID + ":" + string(pdfmodel.RelationTypeRelatesTo) + ":" + c.ID
						if !seen[relID] {
							seen[relID] = true
							relations = append(relations, pdfmodel.Relation{
								ID:         relID,
								SubjectID:  ent.ID,
								Predicate:  pdfmodel.RelationTypeRelatesTo,
								ObjectID:   c.ID,
								Confidence: 0.80,
								ChunkID:    ch.ChunkID,
								Source:     pdfmodel.RelationSourceRuleBased,
							})
						}
					}
				}
			}
		}
	}

	return relations, nil
}
