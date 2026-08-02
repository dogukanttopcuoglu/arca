# 0022: Concept Domain Model & Topic Extraction Architecture

- **Status:** Accepted
- **Date:** 2026-08-02
- **Deciders:** Staff Software Architect & Lead Engineer

## Context and Problem Statement

Following `EntityExtractorPass` (ADR-0021), ARC requires abstract topic and thematic concept discovery capabilities. While `Keyword` represents lexical term frequencies and `Entity` represents concrete named objects (`Person`, `Organization`), `Concept` represents higher-level thematic topics (`"Vector Search Optimization"`, `"Hierarchical Chunking"`). Modeling a full taxonomy or ontology tree at this stage would be over-engineering.

## Decision Drivers

- **Minimal Extensible Domain Model**: Keep `Concept` lean (`ID`, `Name`, `Score`, `Source`), avoiding premature ontology hierarchies (`ParentConceptID`, `Category`) which can be inferred or added in future passes.
- **Narrow Input Strategy Seam**: Decouple `ConceptExtractor` from the full `EnrichmentInput` struct by passing a narrow `ConceptInput` struct (`Tree`, `Chunks`, `Keywords`, `Entities`, `Language`).
- **Explicit Chunk-Level Attachment**: Concepts are attached to document-level metadata and chunk-level metadata based on `SectionPath` and keyword provenance.
- **Contract Safety**: Introduce `CapabilityConcepts` token for `CompositeEnricher` contract validation.

## Decided Options

### Option A: Minimal Concept Domain Model & Narrow Strategy Seam (ACCEPTED)
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

- **Narrow `ConceptInput` & Interface Seam**:
  ```go
  type ConceptInput struct {
      Tree     *pdfmodel.SemanticTree
      Chunks   []pdfmodel.KnowledgeChunk
      Keywords []pdfmodel.Keyword
      Entities []pdfmodel.Entity
      Language string
  }

  type ConceptExtractor interface {
      ExtractConcepts(ctx context.Context, input ConceptInput) ([]pdfmodel.Concept, error)
  }
  ```

- **`ConceptExtractorPass` Pipeline Stage**:
  - Requires: `CapabilityRawMetadata`, `CapabilityLanguage`, `CapabilitySemanticTree`, `CapabilityKeywords`, `CapabilityEntities`
  - Provides: `CapabilityConcepts`

- **Dual-Level Attachment Strategy**:
  - **Document Level**: Attached to `DocumentMetadata.Concepts`.
  - **Chunk Level**: Attached to `KnowledgeChunk.Concepts` when `KnowledgeChunk.SectionPath` matches the concept's originating heading or when chunk text contains the concept terms.

## Consequences

### Positive
- Narrow `ConceptInput` keeps strategy implementations cleanly decoupled from pipeline orchestrators.
- Explicit capability contract ensures all required inputs (`Tree`, `Keywords`, `Entities`) exist before pass execution.
- Extensible foundation for future `RelationExtractorPass` and `SummaryPass`.

### Negative
- Requires maintaining `ConceptInput` struct translation in `ConceptExtractorPass`.
