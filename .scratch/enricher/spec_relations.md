# Technical Specification: Relation Extraction Architecture & SPO Graph Seam

- **Author:** Staff Software Architect & Lead Engineer
- **Status:** Approved
- **Created:** 2026-08-02
- **Target Branch:** `main`

---

## 1. Executive Summary

This specification defines the architecture for directed relationship extraction in ARC's enrichment pipeline. Following ADR-0023, `Relation` is modeled as a lean directed SPO triple (`ID`, `SubjectID`, `Predicate`, `ObjectID`, `Confidence`, `ChunkID`, `Source`) attached to both `DocumentMetadata` and `KnowledgeChunk`. `RelationExtractorPass` delegates to a narrow `RelationExtractor` strategy interface taking `RelationInput`.

---

## 2. Domain Models & Contracts

### 2.1 Domain Models (`internal/pdfinspector/model/relation.go`)
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

### 2.2 Narrow Input & Interface Seam (`internal/pdfinspector/enrichment/relation_extractor.go`)
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

### 2.3 Capability Token (`internal/pdfinspector/enrichment/pass.go`)
```go
const (
    CapabilityRelations Capability = "relations"
)
```

---

## 3. Implementation Plan & Tickets

- **Ticket 13 (`13-relation-domain-model.md`)**: Add `relation.go` with `Relation`, `RelationType`, and `RelationSource` structs, update `DocumentMetadata` and `KnowledgeChunk`, and declare `CapabilityRelations` token.
- **Ticket 14 (`14-rule-based-relation-extractor.md`)**: Implement `RelationExtractor` interface seam with `RelationInput` and `RuleBasedRelationExtractor` extracting co-occurrence & predicate relationships.
- **Ticket 15 (`15-relation-extractor-pass-integration.md`)**: Implement `RelationExtractorPass` with explicit capability contract (`CapabilityEntities`, `CapabilityConcepts`) and integrate into `CompositeEnricher`.
