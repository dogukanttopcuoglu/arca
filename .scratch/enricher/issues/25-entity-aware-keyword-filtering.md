# Ticket #25 — Entity-Aware Keyword Filtering

## Status
`TODO`

## Problem

QA audit on `inspection_result_rick_rubin_v2.json` revealed that while entity extraction correctly identifies compound named entities (`Def Jam Recordings`, `New York`, `Rick Rubin`), the keyword layer remains entity-unaware and continues to produce entity fragment tokens as lexical keywords:

**Current (bad):**
```
Metadata.Keywords:
  - def    (score: 1.0)
  - jam    (score: 1.0)
  - new    (score: 1.0)
  - york   (score: 1.0)
  - recordings (score: 1.0)
```

**Expected (good):**
```
Metadata.Keywords:
  - creative    (score: 0.5)
  - expression  (score: 0.5)
  - founded     (score: 0.5)
  - alongside   (score: 0.5)
  - artistic    (score: 0.5)
```

## Root Cause

`RuleBasedKeywordExtractor` tokenizes raw chunk text independently of `EntityExtractorPass` output. It does not filter out tokens that are sub-tokens of extracted named entities.

## Target Flow

```
Text
  ↓
Entity Extraction   →  EntityInput
  ↓
Keyword Extraction  ←  receives Entities []model.Entity
  ↓
Entity Fragment Filtering
  ↓
Final Clean Keywords
```

## Required Changes

### 1. Update `KeywordInput` struct (`keyword_extractor.go`)

Add `Entities []model.Entity` field so the extractor can build a fragment blocklist:

```go
type KeywordInput struct {
    Chunks   []model.KnowledgeChunk
    Language string
    Entities []model.Entity  // NEW: for entity-aware filtering
}
```

### 2. Update `RuleBasedKeywordExtractor.ExtractKeywords` (`keyword_extractor.go`)

Build a token blocklist from entity names by splitting each entity canonical name into tokens:

```
"Def Jam Recordings" → {"def", "jam", "recordings"}
"New York"           → {"new", "york"}
"Rick Rubin"         → {"rick", "rubin"}
```

Filter any keyword token that appears in this blocklist.

### 3. Update `KeywordExtractorPass.Execute` (`keyword_pass.go`)

Pass `input.Metadata.Entities` into `KeywordInput` after entity extraction has run. Execution order must remain:

```
EntityExtractorPass → KeywordExtractorPass
```

### 4. Update `KeywordExtractor` interface signature (`keyword_extractor.go`)

```go
type KeywordExtractor interface {
    ExtractKeywords(ctx context.Context, input KeywordInput) ([]model.Keyword, error)
}
```

## Acceptance Criteria

- [ ] `Metadata.Keywords` does not contain tokens that are sub-parts of any extracted entity name.
- [ ] `def`, `jam`, `new`, `york` do NOT appear in `Metadata.Keywords` when `Def Jam Recordings` and `New York` are present in `Metadata.Entities`.
- [ ] Multi-word entity names like `Russell Simmons` suppress both `russell` and `simmons` from keywords.
- [ ] Regression test `TestKeywordExtractor_FiltersEntityFragments` added and passing.
- [ ] All existing keyword extractor tests still pass.

## Known Limitations (Out of Scope)

- **Russell Simmons entity recall gap**: `RuleBasedEntityExtractor` does not yet detect `Russell Simmons`. This is a separate recall issue to be benchmarked with GLiNER (Ticket #26).
- **Chunk statistics (`token_estimate`, `character_count` = 0)**: Separate polish ticket (#27).
- **Metadata consistency (`pageCount: 0`, `searchable: false`)**: Separate polish ticket (#28).
