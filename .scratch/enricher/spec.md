# Technical Specification: Enricher Pass Pipeline & Resolver Architecture

- **Author:** Staff Software Architect & Lead Engineer
- **Status:** Approved
- **Created:** 2026-08-02
- **Target Branch:** `main`

---

## 1. Executive Summary

This specification solidifies the `internal/pdfinspector/enrichment` architecture before adding advanced entity extraction and keyword tagging. It replaces the hardcoded book/author fallback heuristics with a chain-of-responsibility `TitleResolver` / `AuthorResolver` pipeline and restructures `Enricher` into a compiler-pass style `CompositeEnricher` that executes distinct `EnricherPass` stages with capability contract validation.

---

## 2. Architectural Design & ADR-0019

### 2.1 Pass Capability Contracts
Each pass declares:
- `Requires()`: Input capabilities needed (e.g., `CapabilityRawMetadata`, `CapabilitySemanticTree`).
- `Provides()`: Capabilities produced (e.g., `CapabilityResolvedTitle`, `CapabilityPageMapping`).

```go
type Capability string

const (
    CapabilityRawMetadata   Capability = "raw_metadata"
    CapabilitySemanticTree  Capability = "semantic_tree"
    CapabilityResolvedTitle Capability = "resolved_title"
    CapabilityResolvedPages Capability = "resolved_pages"
    CapabilityKeywords      Capability = "keywords"
)

type EnricherPass interface {
    Name() string
    Requires() []Capability
    Provides() []Capability
    Execute(ctx context.Context, agg *model.InspectionAggregate) error
}
```

### 2.2 Title & Author Fallback Chain
Title resolution follows a strict priority chain:
1. `PDFMetadataResolver`: Checks explicit PDF metadata header title (if non-generic).
2. `HeadingResolver`: Checks cover / early page headings (pages 1-5).
3. `TOCResolver`: Checks Table of Contents patterns.
4. `LLMResolver`: Optional LLM title resolver.
5. `DefaultUnknownResolver`: Fallback to `"Unknown Title"` (never hardcoded book names).

---

## 3. Implementation Plan & Tickets

- **Ticket 01 (`01-title-author-resolver-chain.md`)**: Implement `TitleResolver` and `AuthorResolver` fallback chains, eradicating hardcoded `"The Creative Act"` and `"Rick Rubin"` strings.
- **Ticket 02 (`02-enricher-pass-pipeline-core.md`)**: Implement `EnricherPass` interface, `Capability` validation, and `CompositeEnricher` pipeline runner.
- **Ticket 03 (`03-refactor-pdfinspector-enrichment.md`)**: Refactor `internal/pdfinspector/enrichment` to use `CompositeEnricher` with `TitleAuthorPass` and `PageResolutionPass`.
