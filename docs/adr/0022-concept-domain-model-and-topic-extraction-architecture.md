# 0022: Concept Domain Model & Topic Extraction Architecture

- **Status:** Accepted
- **Date:** 2026-08-02
- **Deciders:** Staff Software Architect & Lead Engineer

## Context and Problem Statement

Following `EntityExtractorPass` (ADR-0021), ARC requires abstract topic and thematic concept discovery capabilities. While `Keyword` represents lexical term frequencies and `Entity` represents concrete named objects (`Person`, `Organization`), `Concept` represents higher-level thematic topics (`"Vector Search Optimization"`, `"Hierarchical Chunking"`). Modeling a full taxonomy or ontology tree at this stage would be over-engineering.

## Decision Drivers

- **Minimal Extensible Domain Model**: Keep `Concept` lean (`ID`, `Name`, `Score`, `Source`), avoiding premature ontology hierarchies (`ParentConceptID`, `Category`) which can be inferred or added in future passes.
- **Strategy Interface Seam**: Decouple `ConceptExtractorPass` from specific NLP algorithms via `ConceptExtractor` interface seam and `ExtractorRegistry`.
- **Contract Safety**: Introduce `CapabilityConcepts` token for `CompositeEnricher` contract validation.

## Decided Options

### Option A: Minimal Concept Domain Model & Extractor Seam (ACCEPTED)
- **`Concept` Struct**:
  ```go
  type ConceptSource string

  const (
      ConceptSourceRuleBased ConceptSource = "rule_based"
      ConceptSourceLLM       ConceptSource = "llm"
      ConceptSourceHybrid    ConceptSource = "hybrid"
  )

  type Concept struct {
      ID     string        `json:"id"`
      Name   string        `json:"name"`
      Score  float64       `json:"score"`
      Source ConceptSource `json:"source"`
  }
  ```
- **`ConceptExtractor` Strategy Interface Seam**:
  ```go
  type ConceptExtractor interface {
      ExtractConcepts(ctx context.Context, input *EnrichmentInput, lang string) ([]pdfmodel.Concept, error)
  }
  ```
- **`ConceptExtractorPass` Pipeline Stage**:
  - Requires: `CapabilityRawMetadata`, `CapabilityLanguage`, `CapabilityKeywords`, `CapabilityEntities`
  - Provides: `CapabilityConcepts`
- **Dual-Level Attachment**:
  - Attached to both `DocumentMetadata.Concepts` and `KnowledgeChunk.Concepts`.

## Consequences

### Positive
- Avoids premature over-engineering or complex ontology trees.
- Deterministic `RuleBasedConceptExtractor` MVP synthesizes section headings and top keywords without external API dependencies.
- Extensible foundation for future `RelationExtractorPass` and `SummaryPass`.

### Negative
- Ontology hierarchy relationships must be inferred or deferred to future dedicated taxonomy passes.
