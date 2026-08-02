# Technical Specification: Entity Extraction Architecture & Mention Seam

- **Author:** Staff Software Architect & Lead Engineer
- **Status:** Approved
- **Created:** 2026-08-02
- **Target Branch:** `main`

---

## 1. Executive Summary

This specification defines the architecture for Named Entity Recognition (NER) in ARC's enrichment pipeline. Following ADR-0021, entity mention discovery is decoupled from canonicalization via a dedicated `EntityExtractor` strategy interface returning `[]EntityMention`. `EntityExtractorPass` receives these mentions, attaches them to individual `KnowledgeChunk` instances, and performs minimal in-memory deduplication to populate `DocumentMetadata.Entities`.

---

## 2. Domain Models & Contracts

### 2.1 Domain Models (`internal/pdfinspector/model/model.go`)
```go
type EntityType string

const (
    EntityTypePerson       EntityType = "person"
    EntityTypeOrganization EntityType = "organization"
    EntityTypeLocation     EntityType = "location"
    EntityTypeProduct      EntityType = "product"
    EntityTypeEvent        EntityType = "event"
    EntityTypeMisc         EntityType = "miscellaneous"
)

type EntityMention struct {
    Text       string     `json:"text"`
    Type       EntityType `json:"type"`
    ChunkID    string     `json:"chunk_id"`
    Confidence float64    `json:"confidence"`
}

type Entity struct {
    ID       string          `json:"id"`
    Name     string          `json:"name"`
    Type     EntityType      `json:"type"`
    Aliases  []string        `json:"aliases,omitempty"`
    Mentions []EntityMention `json:"mentions,omitempty"`
    Score    float64         `json:"score"`
}
```

### 2.2 Interface Seam (`internal/pdfinspector/enrichment/entity_extractor.go`)
```go
type EntityExtractor interface {
    ExtractEntities(ctx context.Context, chunks []pdfmodel.KnowledgeChunk, lang string) ([]pdfmodel.EntityMention, error)
}
```

### 2.3 Capability Token (`internal/pdfinspector/enrichment/pass.go`)
```go
const (
    CapabilityEntities Capability = "entities"
)
```

---

## 3. Implementation Plan & Tickets

- **Ticket 07 (`07-entity-domain-models.md`)**: Add `EntityType`, `EntityMention`, `Entity` structs to `model.go` and add fields to `DocumentMetadata` and `KnowledgeChunk`.
- **Ticket 08 (`08-rule-based-entity-extractor.md`)**: Implement `EntityExtractor` interface seam and `RuleBasedEntityExtractor` for TR/EN mention extraction.
- **Ticket 09 (`09-entity-extractor-pass-integration.md`)**: Implement `EntityExtractorPass` capability contract and integrate into `CompositeEnricher`.
