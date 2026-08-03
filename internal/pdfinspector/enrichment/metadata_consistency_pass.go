package enrichment

import (
	"context"
)

// MetadataConsistencyPass implements EnricherPass for resolving missing or inconsistent
// document-level metadata fields that may be absent when EnrichmentInput arrives from
// sources other than the full inspector pipeline (e.g., test fixtures, importers).
//
// Fields resolved:
//
//   - PageCount: derived from len(PageMap) or the highest page number in chunks,
//     only if PageCount == 0.
//   - Searchable: set to true if PageMap contains non-empty markdown text,
//     only if Searchable == false and PageMap is non-empty.
//
// Values already set by the inspector (buildDocumentMetadata) are never overwritten.
type MetadataConsistencyPass struct{}

// NewMetadataConsistencyPass constructs a MetadataConsistencyPass instance.
func NewMetadataConsistencyPass() *MetadataConsistencyPass {
	return &MetadataConsistencyPass{}
}

func (p *MetadataConsistencyPass) Name() string { return "MetadataConsistencyPass" }
func (p *MetadataConsistencyPass) Requires() []Capability {
	return []Capability{CapabilityRawMetadata}
}
func (p *MetadataConsistencyPass) Provides() []Capability {
	return []Capability{CapabilityMetadataConsistency}
}

// Execute resolves PageCount and Searchable from available structural signals.
func (p *MetadataConsistencyPass) Execute(ctx context.Context, input *EnrichmentInput) ([]string, error) {
	if input == nil || input.Metadata == nil {
		return nil, nil
	}

	meta := input.Metadata

	// --- PageCount resolution ---
	// If PageCount is 0, derive from PageMap length or the highest chunk page number.
	if meta.PageCount == 0 {
		maxPage := len(input.PageMap)
		for _, ch := range input.Chunks {
			for _, p := range ch.PageNumbers {
				if p > maxPage {
					maxPage = p
				}
			}
		}
		if maxPage > 0 {
			meta.PageCount = maxPage
		}
	}

	// --- Searchable resolution ---
	// If Searchable is false but PageMap contains readable markdown, mark as searchable.
	if !meta.Searchable && len(input.PageMap) > 0 {
		for _, pm := range input.PageMap {
			if pm.Markdown != "" {
				meta.Searchable = true
				break
			}
		}
	}

	return nil, nil
}
