package enrichment

import (
	"context"
	"strings"

	pdfmodel "arca/internal/pdfinspector/model"
)

// HybridEntityExtractor merges results from a high-precision primary extractor
// and a high-recall secondary extractor using confidence-based deduplication.
//
// Design contract:
//   - primary (RuleBasedEntityExtractor): always runs, no network dependency, high precision.
//   - secondary (GLiNEREntityExtractor): network-dependent, high recall, errors silently absorbed.
//   - Merge key: (chunkID, entityType, normalizedText)
//   - On collision: higher confidence wins.
//   - On secondary failure: primary-only output is returned (graceful degradation).
//
// This implements the EntityExtractor interface and plugs directly into
// NewEnricherWithExtractors for dependency injection.
type HybridEntityExtractor struct {
	primary   EntityExtractor
	secondary EntityExtractor
}

// NewHybridEntityExtractor constructs a HybridEntityExtractor.
//
//	primary   = RuleBasedEntityExtractor (fast, deterministic, no network)
//	secondary = GLiNEREntityExtractor    (zero-shot NER, network-dependent)
func NewHybridEntityExtractor(primary, secondary EntityExtractor) *HybridEntityExtractor {
	return &HybridEntityExtractor{
		primary:   primary,
		secondary: secondary,
	}
}

// ExtractEntities runs both extractors and returns a merged, deduplicated mention list.
//
// Merge strategy:
//  1. Primary always runs — no network, high precision, must not fail.
//  2. Secondary runs — errors silently absorbed (service may be unavailable).
//  3. Union: secondary entities not found by primary are added.
//  4. On overlap: higher confidence mention wins.
func (h *HybridEntityExtractor) ExtractEntities(ctx context.Context, input EntityInput) ([]pdfmodel.EntityMention, error) {
	primaryMentions, err := h.primary.ExtractEntities(ctx, input)
	if err != nil {
		return nil, err
	}

	// Secondary errors are silently absorbed — GLiNER may be unavailable.
	secondaryMentions, _ := h.secondary.ExtractEntities(ctx, input)

	return mergeMentions(primaryMentions, secondaryMentions), nil
}

// mergeMentions performs union deduplication over two EntityMention slices.
// Key: (chunkID, entityType, normalizedText). On collision, higher confidence wins.
func mergeMentions(primary, secondary []pdfmodel.EntityMention) []pdfmodel.EntityMention {
	type mentionKey struct {
		chunkID string
		label   pdfmodel.EntityType
		text    string
	}

	merged := make(map[mentionKey]pdfmodel.EntityMention, len(primary)+len(secondary))

	for _, m := range primary {
		key := mentionKey{m.ChunkID, m.Type, strings.ToLower(strings.TrimSpace(m.Text))}
		merged[key] = m
	}

	for _, m := range secondary {
		key := mentionKey{m.ChunkID, m.Type, strings.ToLower(strings.TrimSpace(m.Text))}
		if existing, ok := merged[key]; ok {
			if m.Confidence > existing.Confidence {
				merged[key] = m
			}
		} else {
			merged[key] = m
		}
	}

	result := make([]pdfmodel.EntityMention, 0, len(merged))
	for _, m := range merged {
		result = append(result, m)
	}
	return result
}
