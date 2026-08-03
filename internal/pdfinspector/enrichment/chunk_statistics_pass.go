package enrichment

import (
	"context"
)

// ChunkStatisticsPass implements EnricherPass for populating chunk-level statistics.
//
// It fills in zero-valued TokenEstimate and CharacterCount fields on KnowledgeChunks.
// Values already populated by the chunking builder are never overwritten, ensuring
// the pass is idempotent and safe to run in any pipeline configuration.
//
// Token estimation formula:
//
//	tokens ≈ characters / 4 (GPT/Claude family approximation)
//
// For production-grade token counting, replace with a real tokenizer (e.g. tiktoken).
type ChunkStatisticsPass struct{}

// NewChunkStatisticsPass constructs a ChunkStatisticsPass instance.
func NewChunkStatisticsPass() *ChunkStatisticsPass {
	return &ChunkStatisticsPass{}
}

func (p *ChunkStatisticsPass) Name() string { return "ChunkStatisticsPass" }
func (p *ChunkStatisticsPass) Requires() []Capability {
	return []Capability{CapabilityRawMetadata}
}
func (p *ChunkStatisticsPass) Provides() []Capability {
	return []Capability{CapabilityChunkStats}
}

// Execute populates CharacterCount and TokenEstimate for any chunk where these
// are zero but ContentMarkdown is non-empty.
func (p *ChunkStatisticsPass) Execute(ctx context.Context, input *EnrichmentInput) ([]string, error) {
	if input == nil {
		return nil, nil
	}

	for i := range input.Chunks {
		ch := &input.Chunks[i]
		if ch.ContentMarkdown == "" {
			continue
		}

		// Never overwrite values already set by the chunking builder.
		if ch.CharacterCount == 0 {
			ch.CharacterCount = len(ch.ContentMarkdown)
		}

		if ch.TokenEstimate == 0 {
			estimated := ch.CharacterCount / 4
			if estimated < 1 {
				estimated = 1
			}
			ch.TokenEstimate = estimated
		}
	}

	return nil, nil
}
