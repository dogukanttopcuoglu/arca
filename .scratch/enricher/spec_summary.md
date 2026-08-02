# Technical Specification: Summary Extraction Architecture & Extractive Strategy Seam

- **Author:** Staff Software Architect & Lead Engineer
- **Status:** Approved
- **Created:** 2026-08-02
- **Target Branch:** `main`

---

## 1. Executive Summary

This specification defines the architecture for extractive document and chunk summarization in ARC's enrichment pipeline. Following ADR-0024, `Summary` is modeled as a lean struct (`Text`, `Source`) attached to both `DocumentMetadata` and `KnowledgeChunk`. `SummaryPass` delegates to a pure producer `SummaryExtractor` strategy interface returning `SummaryResult`.

---

## 2. Domain Models & Contracts

### 2.1 Domain Models (`internal/pdfinspector/model/summary.go`)
```go
type SummarySource string

const (
    SummarySourceRuleBased SummarySource = "rule_based"
    SummarySourceLLM       SummarySource = "llm"
    SummarySourceHybrid    SummarySource = "hybrid"
)

type Summary struct {
    Text   string        `json:"text"`
    Source SummarySource `json:"source"`
}
```

### 2.2 Narrow Input & Interface Seam (`internal/pdfinspector/enrichment/summary_extractor.go`)
```go
type SummaryInput struct {
    Chunks    []pdfmodel.KnowledgeChunk
    Keywords  []pdfmodel.Keyword
    Entities  []pdfmodel.Entity
    Concepts  []pdfmodel.Concept
    Relations []pdfmodel.Relation
}

type SummaryResult struct {
    DocumentSummary *pdfmodel.Summary
    ChunkSummaries  map[string]*pdfmodel.Summary
}

type SummaryExtractor interface {
    ExtractSummaries(ctx context.Context, input SummaryInput) (SummaryResult, error)
}
```

### 2.3 Capability Token (`internal/pdfinspector/enrichment/pass.go`)
```go
const (
    CapabilitySummary Capability = "summary"
)
```

---

## 3. Implementation Plan & Tickets

- **Ticket 16 (`16-summary-domain-model.md`)**: Add `summary.go` with `Summary` and `SummarySource` structs, update `DocumentMetadata` and `KnowledgeChunk`, and declare `CapabilitySummary` token.
- **Ticket 17 (`17-rule-based-summary-extractor.md`)**: Implement `SummaryExtractor` interface seam with `SummaryInput`, `SummaryResult`, and extractive `RuleBasedSummaryExtractor`.
- **Ticket 18 (`18-summary-pass-integration.md`)**: Implement `SummaryPass` with explicit capability contract (`CapabilityKeywords`, `CapabilityEntities`, `CapabilityConcepts`, `CapabilityRelations`) and integrate into `CompositeEnricher`.
