# 0021: Entity Extraction Architecture & Mention Seam

- **Status:** Accepted
- **Date:** 2026-08-02
- **Deciders:** Staff Software Architect & Lead Engineer

## Context and Problem Statement

Following `KeywordExtractorPass` (ADR-0020), ARC requires Named Entity Recognition (NER) capabilities (`Person`, `Organization`, `Location`, `Product`, `Event`, `Misc`). Unlike un-typed lexical keywords, entities possess typed classifications and provenance surface mentions across chunks. Mixing raw mention discovery with complex entity canonicalization and alias resolution in a single pass would violate the Single Responsibility Principle (SRP).

## Decision Drivers

- **Extraction vs. Canonicalization Seam**: Extractors must only discover surface mentions (`EntityMention`), leaving entity aggregation to pass orchestration.
- **Pluggable Extractor Strategy**: `EntityExtractorPass` delegates mention discovery to an `EntityExtractor` interface seam and registry.
- **Minimal MVP Canonicalization**: Full alias resolution and cross-document canonicalization are explicitly deferred to a future `EntityCanonicalizerPass`. The initial MVP pass performs only basic in-memory mention grouping to populate document-level entity lists.
- **Contract Safety**: Introduces `CapabilityEntities` token to enforce compiler-pass dependencies.

## Decided Options

### Option A: Mention-Based Extractor Seam & Minimal MVP Pass Aggregation (ACCEPTED)
- **`EntityExtractor` Interface Seam**:
  ```go
  type EntityExtractor interface {
      ExtractEntities(ctx context.Context, chunks []pdfmodel.KnowledgeChunk, lang string) ([]pdfmodel.EntityMention, error)
  }
  ```
- **`EntityExtractorPass` Responsibility**:
  - Requires: `CapabilityRawMetadata`, `CapabilityLanguage`, `CapabilitySemanticTree`
  - Provides: `CapabilityEntities`
  - Calls `EntityExtractor` strategy to obtain raw `[]EntityMention`.
  - Attaches `EntityMention` slice to individual `KnowledgeChunk.Entities`.
  - Performs minimal in-memory grouping of mentions by text/type to populate `DocumentMetadata.Entities`.

### Option B: Monolithic Entity Extractor returning Canonical Entities (REJECTED)
Having extractors attempt full canonicalization, alias merging, and entity linking in a single step.

## Consequences

### Positive
- Strict SRP separation: Extractor strategies only extract mentions; pass handles document aggregation.
- 100% deterministic `RuleBasedEntityExtractor` MVP for fast, reliable unit testing.
- Clean foundation for future `EntityCanonicalizerPass`, `RelationExtractorPass`, and Knowledge Graph indexing.

### Negative
- Requires distinct domain models for `EntityMention` vs. `Entity`.
