# Ticket #27 — Chunk Statistics Pass: Populate token_estimate & character_count

## Status
`TODO`

## Problem

QA audit on `inspection_result_rick_rubin_v2.json` revealed:

```json
"token_estimate": 0,
"character_count": 0
```

These fields are part of the `KnowledgeChunk` contract and are required by downstream
retrieval and indexing systems for:
- Context window budget management
- Chunk ranking and scoring
- Token-aware splitting decisions

## Root Cause

`chunking/builder.go` correctly populates `TokenEstimate` and `CharacterCount` for
chunks it creates. However, when chunks arrive at the enrichment pipeline from any
other source (test fixtures, external importers, schema migrations), these fields
may be zero.

The enrichment pipeline has no pass that ensures these fields are populated before
downstream passes consume the chunks.

## Solution

Add a `ChunkStatisticsPass` to the enrichment pipeline that:
1. Computes `CharacterCount` = `len(chunk.ContentMarkdown)`
2. Estimates `TokenEstimate` ≈ `len(chunk.ContentMarkdown) / 4` (GPT-family approximation)
3. Only fills in zero values — never overwrites values already set by the chunker.

### Token Estimation Formula

The standard approximation used across GPT/Claude models:
```
tokens ≈ characters / 4
```

This is intentionally conservative — actual tokenization is model-specific.
For production, a real tokenizer (e.g. `tiktoken`) should be used via GLiNER service.

## Target Flow

```
ChunkingBuilder (real pipeline) → TokenEstimate populated ✅
Test fixture / external input   → TokenEstimate = 0 ❌
                ↓
        ChunkStatisticsPass
                ↓
        TokenEstimate populated ✅ (always)
```

## Required Changes

### 1. Create `chunk_statistics_pass.go`

New `EnricherPass` that fills in zero `TokenEstimate` and `CharacterCount`:

```go
type ChunkStatisticsPass struct{}

func (p *ChunkStatisticsPass) Execute(ctx context.Context, input *EnrichmentInput) error {
    for i := range input.Chunks {
        ch := &input.Chunks[i]
        if ch.CharacterCount == 0 {
            ch.CharacterCount = len(ch.ContentMarkdown)
        }
        if ch.TokenEstimate == 0 && ch.CharacterCount > 0 {
            ch.TokenEstimate = max(1, ch.CharacterCount/4)
        }
    }
    return nil
}
```

### 2. Register in `enricher.go` CompositeEnricher

Run early — before any pass that might use token counts for budget decisions.

### 3. Define `CapabilityChunkStats` capability constant

```go
CapabilityChunkStats Capability = "chunk_stats"
```

## Acceptance Criteria

- [ ] After enrichment, all chunks with non-empty `ContentMarkdown` have `token_estimate > 0`
- [ ] After enrichment, all chunks with non-empty `ContentMarkdown` have `character_count > 0`
- [ ] Values already set by `chunking/builder.go` are NOT overwritten
- [ ] Regression test `TestChunkStatisticsPass` added and passing
- [ ] All existing tests still pass
