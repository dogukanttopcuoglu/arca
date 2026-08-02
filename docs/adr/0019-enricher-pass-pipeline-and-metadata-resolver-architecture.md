# 0019: Enricher Pass Pipeline & Metadata Resolver Architecture

- **Status:** Accepted
- **Date:** 2026-08-02
- **Deciders:** Staff Software Architect & Lead Engineer

## Context and Problem Statement

The post-extraction metadata enrichment stage in `internal/pdfinspector/enrichment` currently relies on hardcoded string heuristics (e.g. fallback book titles like `"The Creative Act: A Way of Being"` and hardcoded author names `"Rick Rubin"`) and a monolithic `DefaultEnricher` implementation. As ARC scales to support Entity Extraction, Concept Linking, Keyword Extraction, and Knowledge Graph indexing, a single monolithic enricher would bloat and become unmaintainable.

## Decision Drivers

- **Compiler Pass Pattern**: Enricher passes must operate like compiler passes with explicit input dependencies (`Requires()`) and outputs (`Provides()`).
- **Resilient Fallback Chains**: Document title and author resolution must follow a fallback chain (`PDF Metadata` -> `Heading Resolver` -> `TOC Resolver` -> `LLM Resolver` -> `"Unknown Title"`), never hardcoding specific book titles into generic domain logic.
- **Deep Seams & Locality**: Adding new enrichment stages (e.g., Entity Extraction, Keyword Tagging) must not require modifying existing passes or orchestrators.

## Decided Options

### Option A: Compiler Pass Pipeline & Resolver Chain (ACCEPTED)
- **`EnricherPass` Interface Seam**:
  ```go
  type EnricherPass interface {
      Name() string
      Requires() []Capability
      Provides() []Capability
      Execute(ctx context.Context, agg *InspectionAggregate) error
  }
  ```
- **`CompositeEnricher` Pipeline**: Executes an ordered slice of `EnricherPass` implementations, verifying capability contracts before execution.
- **`TitleResolver` / `AuthorResolver` Fallback Chain**:
  - `PDFMetadataResolver`
  - `HeadingResolver`
  - `TOCResolver`
  - `LLMResolver` (Optional)
  - `DefaultUnknownResolver` (Fallback to `"Unknown Document"`, never hardcoded titles).

### Option B: Monolithic Enricher Struct (REJECTED)
Continuing with a single struct executing all title resolution, page mapping, entity extraction, and keyword tagging inline.

## Consequences

### Positive
- Adding new enrichment capabilities (Entity Extraction, Concept Linking, Summaries) is as simple as adding a new `EnricherPass` adapter to `CompositeEnricher`.
- Eradicates hardcoded book/author titles from generic domain logic.
- Clear compiler-pass style capability contracts (`Requires` vs `Provides`).

### Negative
- Requires maintaining pass dependency verification logic.
