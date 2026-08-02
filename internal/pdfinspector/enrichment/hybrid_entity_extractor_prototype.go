// PROTOTYPE — HybridEntityExtractor
// THROWAWAY CODE — Do not merge to main without validation.
// Question: Does merging RuleBased + GLiNER outputs improve entity recall?
//
// This file lives on branch: prototype/gliner-entity-extraction
// Context pointer: .scratch/enricher/issues/26-gliner-entity-recall-benchmark.md

package enrichment

import (
	"context"
	"strings"

	pdfmodel "arca/internal/pdfinspector/model"
)

// HybridEntityExtractor merges results from a primary (high-precision rule-based)
// and a secondary (high-recall GLiNER) extractor.
// Deduplication uses (label, normalized-text) as the merge key.
// When both extractors agree on an entity, the higher confidence wins.
type HybridEntityExtractor struct {
	primary   EntityExtractor // RuleBasedEntityExtractor
	secondary EntityExtractor // GLiNEREntityExtractor
}

// NewHybridEntityExtractor constructs a HybridEntityExtractor.
// primary   = RuleBasedEntityExtractor (fast, deterministic, no network)
// secondary = GLiNEREntityExtractor    (zero-shot NER, network-dependent)
func NewHybridEntityExtractor(primary, secondary EntityExtractor) *HybridEntityExtractor {
	return &HybridEntityExtractor{
		primary:   primary,
		secondary: secondary,
	}
}

// ExtractEntities runs both extractors and merges results.
// Merge strategy:
//  1. Run primary extractor — always succeeds (no network).
//  2. Run secondary extractor — may fail/fallback silently.
//  3. Union: add secondary entities not already found by primary.
//  4. On overlap: keep the higher-confidence mention.
func (h *HybridEntityExtractor) ExtractEntities(ctx context.Context, input EntityInput) ([]pdfmodel.EntityMention, error) {
	primaryMentions, err := h.primary.ExtractEntities(ctx, input)
	if err != nil {
		// Primary failure is fatal — it has no network dependency, should not fail.
		return nil, err
	}

	secondaryMentions, _ := h.secondary.ExtractEntities(ctx, input)
	// Secondary errors are silently absorbed — GLiNER may be unavailable.

	return mergeMentions(primaryMentions, secondaryMentions), nil
}

// mergeMentions performs union deduplication of two EntityMention slices.
// Key: (chunkID, entityType, normalizedText)
// When keys collide, the higher-confidence mention wins.
func mergeMentions(primary, secondary []pdfmodel.EntityMention) []pdfmodel.EntityMention {
	type mentionKey struct {
		chunkID string
		label   pdfmodel.EntityType
		text    string
	}

	merged := make(map[mentionKey]pdfmodel.EntityMention)

	for _, m := range primary {
		key := mentionKey{m.ChunkID, m.Type, strings.ToLower(strings.TrimSpace(m.Text))}
		merged[key] = m
	}

	for _, m := range secondary {
		key := mentionKey{m.ChunkID, m.Type, strings.ToLower(strings.TrimSpace(m.Text))}
		if existing, ok := merged[key]; ok {
			// Keep higher confidence
			if m.Confidence > existing.Confidence {
				merged[key] = m
			}
		} else {
			// Net-new entity from GLiNER — add to result
			merged[key] = m
		}
	}

	result := make([]pdfmodel.EntityMention, 0, len(merged))
	for _, m := range merged {
		result = append(result, m)
	}
	return result
}
