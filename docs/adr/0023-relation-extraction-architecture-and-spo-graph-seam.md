# 0023: Relation Extraction Architecture & SPO Graph Seam

- **Status:** Accepted
- **Date:** 2026-08-02
- **Deciders:** Staff Software Architect & Lead Engineer

## Context and Problem Statement

Following `EntityExtractorPass` (ADR-0021) and `ConceptExtractorPass` (ADR-0022), ARC requires relationship discovery capabilities to connect `Entities` and `Concepts` into a directed Knowledge Graph. The relation model must support GraphRAG traversals without caching redundant metadata or over-complicating early taxonomy hierarchies.

## Decision Drivers

- **Minimal Directed SPO Model**: Model directed Subject-Predicate-Object triples (`SubjectID`, `Predicate`, `ObjectID`, `Confidence`, `ChunkID`, `Source`).
- **Standardized Node Identifiers**: Use explicit prefixes (`entity:...`, `concept:...`) for robust graph node referencing.
- **Narrow Input Strategy Seam**: Decouple `RelationExtractor` strategies from pipeline orchestrators via narrow `RelationInput` struct (`Chunks`, `Entities`, `Concepts`).
- **Deterministic ID Deduplication**: Generate relation IDs deterministically via `rel:subjectID:predicate:objectID` hash to automate document-level canonical catalog deduplication.
- **Scoped MVP Relationships**: Focus MVP strictly on `Entity ↔ Entity` and `Entity ↔ Concept` relations, deferring `Concept ↔ Concept` taxonomy graphs.

## Decided Options

### Option A: Minimal Directed SPO Model & Narrow Strategy Seam (ACCEPTED)

- **`Relation` & Enums**:
  ```go
  type RelationSource string

  const (
      RelationSourceRuleBased RelationSource = "rule_based"
      RelationSourceLLM       RelationSource = "llm"
      RelationSourceHybrid    RelationSource = "hybrid"
  )

  type RelationType string

  const (
      RelationTypeFoundedBy  RelationType = "founded_by"
      RelationTypeLocatedIn  RelationType = "located_in"
      RelationTypePartOf     RelationType = "part_of"
      RelationTypeRelatesTo  RelationType = "relates_to"
      RelationTypeAuthorOf   RelationType = "author_of"
      RelationTypeAssociated RelationType = "associated_with"
  )

  type Relation struct {
      ID         string         `json:"id"`
      SubjectID  string         `json:"subject_id"`
      Predicate  RelationType   `json:"predicate"`
      ObjectID   string         `json:"object_id"`
      Confidence float64        `json:"confidence"`
      ChunkID    string         `json:"chunk_id,omitempty"`
      Source     RelationSource `json:"source"`
  }
  ```

- **Narrow `RelationInput` & Interface Seam**:
  ```go
  type RelationInput struct {
      Chunks   []pdfmodel.KnowledgeChunk
      Entities []pdfmodel.Entity
      Concepts []pdfmodel.Concept
  }

  type RelationExtractor interface {
      ExtractRelations(ctx context.Context, input RelationInput) ([]pdfmodel.Relation, error)
  }
  ```

- **`RelationExtractorPass` Pipeline Stage**:
  - Requires: `CapabilityEntities`, `CapabilityConcepts`
  - Provides: `CapabilityRelations`

- **Dual-Level Attachment Strategy**:
  - **Document Level**: Canonical, deduplicated catalog on `DocumentMetadata.Relations`.
  - **Chunk Level**: Chunk-specific relation instances on `KnowledgeChunk.Relations`.

## Consequences

### Positive
- Directed SPO structure provides zero-overhead foundation for GraphRAG and 3-way graph traversals.
- Deterministic relation IDs eliminate duplicate edges automatically.
- Decoupled `RelationInput` keeps strategy implementations independent from pipeline orchestrators.

### Negative
- `Concept ↔ Concept` taxonomy relationships are explicitly deferred to future passes.
