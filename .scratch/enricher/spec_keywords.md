# Technical Specification: Language Detection & Keyword Extractor Architecture

- **Author:** Staff Software Architect & Lead Engineer
- **Status:** Approved
- **Created:** 2026-08-02
- **Target Branch:** `main`

---

## 1. Executive Summary

This specification defines the first production AI/NLP passes for ARC's enrichment pipeline: `LanguageDetectionPass` and `KeywordExtractorPass`. It establishes a pluggable `KeywordExtractor` interface seam and `RuleBasedKeywordExtractor` MVP with language-aware stopword filtering, attaching structured `Keyword` objects to both `DocumentMetadata` and individual `KnowledgeChunk` instances.

---

## 2. Structured Domain Models

```go
type KeywordSource string

const (
    KeywordSourceRuleBased KeywordSource = "rule_based"
    KeywordSourceLLM       KeywordSource = "llm"
    KeywordSourceHybrid    KeywordSource = "hybrid"
)

type Keyword struct {
    Value    string        `json:"value"`
    Score    float64       `json:"score"`
    Source   KeywordSource `json:"source"`
    ChunkIDs []string      `json:"chunk_ids,omitempty"`
}
```

---

## 3. Pass Sequence & Capabilities

1. **`LanguageDetectionPass`**:
   - Requires: `CapabilityRawMetadata`
   - Provides: `CapabilityLanguage`
   - Detects `"tr"` vs `"en"` language codes.

2. **`KeywordExtractorPass`**:
   - Requires: `CapabilityLanguage`, `CapabilitySemanticTree`
   - Provides: `CapabilityKeywords`
   - Executes `KeywordExtractor` strategy (defaulting to `RuleBasedKeywordExtractor`).

---

## 4. Implementation Plan & Tickets

- **Ticket 01 (`04-keyword-domain-model-and-language-detection.md`)**: Add `Keyword` struct to `internal/pdfinspector/model` and implement `LanguageDetectionPass`.
- **Ticket 02 (`05-rule-based-keyword-extractor.md`)**: Implement `KeywordExtractor` interface seam, `RuleBasedKeywordExtractor` with TR/EN stopword filtering, and unit tests.
- **Ticket 03 (`06-keyword-extractor-pass-integration.md`)**: Implement `KeywordExtractorPass` attaching keywords to document & chunks, integrated into `CompositeEnricher`.
