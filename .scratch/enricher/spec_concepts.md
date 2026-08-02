# Technical Specification: Concept Domain Model & Topic Extraction Architecture

- **Author:** Staff Software Architect & Lead Engineer
- **Status:** Approved
- **Created:** 2026-08-02
- **Target Branch:** `main`

---

## 1. Executive Summary

This specification defines the architecture for abstract topic and concept discovery in ARC's enrichment pipeline. Following ADR-0022, `Concept` is modeled as a lean struct (`ID`, `Name`, `Score`, `Source`) attached to both `DocumentMetadata` and `KnowledgeChunk`. `ConceptExtractorPass` delegates to a narrow `ConceptExtractor` strategy interface taking `ConceptInput`.

---

## 2. Domain Models & Contracts

### 2.1 Domain Models (`internal/pdfinspector/model/concept.go`)
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

### 2.2 Narrow Input & Interface Seam (`internal/pdfinspector/enrichment/concept_extractor.go`)
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

### 2.3 Capability Token (`internal/pdfinspector/enrichment/pass.go`)
```go
const (
    CapabilityConcepts Capability = "concepts"
)
```

---

## 3. Implementation Plan & Tickets

- **Ticket 10 (`10-concept-domain-model.md`)**: Add `concept.go` with `Concept` and `ConceptSource` structs, update `DocumentMetadata` and `KnowledgeChunk`, and declare `CapabilityConcepts` token.
- **Ticket 11 (`11-rule-based-concept-extractor.md`)**: Implement `ConceptExtractor` interface seam with `ConceptInput` and `RuleBasedConceptExtractor` synthesizing section headings and key phrases.
- **Ticket 12 (`12-concept-extractor-pass-integration.md`)**: Implement `ConceptExtractorPass` with explicit capability contract (`CapabilitySemanticTree`, `CapabilityKeywords`, `CapabilityEntities`) and integrate into `CompositeEnricher`.
