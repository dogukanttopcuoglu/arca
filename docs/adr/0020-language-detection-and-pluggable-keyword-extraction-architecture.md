# 0020: Language Detection & Pluggable Keyword Extraction Architecture

- **Status:** Accepted
- **Date:** 2026-08-02
- **Deciders:** Staff Software Architect & Lead Engineer

## Context and Problem Statement

The post-extraction enrichment pipeline requires language detection and keyword extraction capabilities. Coupling `KeywordExtractorPass` directly to a specific LLM or single algorithm would reduce flexibility and make testing non-deterministic. Furthermore, downstream passes (Stopword filtering, Tokenization, Entity Extraction, Prompting) require knowing the document language upfront.

## Decision Drivers

- **Language Primacy**: `LanguageDetectionPass` must run early to inform downstream tokenization, stopword lists, and prompting.
- **Pluggable Keyword Extractor Seam**: `KeywordExtractorPass` must delegate keyword generation to a `KeywordExtractor` interface seam and `ExtractorRegistry`, supporting `rule_based`, `llm`, and `hybrid` implementations.
- **Hierarchical Keyword Locality**: Keywords must be attached to both `DocumentMetadata.Keywords` (document-level) and `KnowledgeChunk.Keywords` (chunk-level for precision RAG retrieval).
- **Structured Keyword Domain Model**: Keywords are represented as structured objects (`Value`, `Score`, `Source`, `ChunkIDs`), not plain strings.

## Decided Options

### Option A: Pluggable Extractor Seam, Deterministic Rule-Based MVP & Language Detection (ACCEPTED)
- **`LanguageDetectionPass`**:
  - Detects ISO language code (e.g., `"tr"`, `"en"`) and updates `DocumentMetadata.Language`.
  - Capability: Provides `CapabilityLanguage`.
- **`KeywordExtractor` Interface Seam & Registry**:
  ```go
  type KeywordExtractor interface {
      Extract(ctx context.Context, chunks []model.KnowledgeChunk, lang string) ([]model.Keyword, error)
  }
  ```
- **`RuleBasedKeywordExtractor` MVP**:
  - Deterministic TF-IDF / term-frequency extraction with language-aware stopword filtering.
  - Zero external API dependencies, 100% fast & deterministic unit tests.
- **Structured `Keyword` Model**:
  ```go
  type Keyword struct {
      Value    string        `json:"value"`
      Score    float64       `json:"score"`
      Source   KeywordSource `json:"source"`
      ChunkIDs []string      `json:"chunk_ids,omitempty"`
  }
  ```

## Consequences

### Positive
- `LanguageDetectionPass` ensures all downstream passes know the document language.
- Deterministic `RuleBasedKeywordExtractor` makes unit testing fast, reliable, and regression-free.
- Chunk-level keywords enable fine-grained sparse keyword filtering during RAG retrieval.
- Easy to swap to `llm` or `hybrid` extractors via configuration without touching pipeline orchestrator code.
