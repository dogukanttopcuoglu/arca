# Specification: Pluggable Enrichment Provider Architecture

* **Status:** Specification
* **Date:** 2026-08-02
* **Target Package:** `internal/pdfinspector/enrichment`
* **ADR Reference:** [ADR-0025](file:///c:/Users/Dogukan/Desktop/arca/docs/adr/0025-pluggable-enrichment-provider-architecture-and-gliner-integration-strategy.md)

---

## 1. Current Architectural State

The ARC enrichment pipeline processes document content through sequential passes in `CompositeEnricher`:

```
LanguageDetectionPass -> TitleAuthorPass -> PageResolutionPass -> KeywordExtractorPass -> EntityExtractorPass -> ConceptExtractorPass -> RelationExtractorPass -> SummaryPass
```

Passes consume and modify `EnrichmentInput`:
- `EntityExtractorPass` delegates entity extraction to `EntityExtractor`.
- `ConceptExtractorPass` delegates concept extraction to `ConceptExtractor`.
- `RelationExtractorPass` delegates relation extraction to `RelationExtractor`.

While the strategy seams exist, the concrete implementations currently lack strict boundary enforcement and provider-based flexibility.

---

## 2. Problem Analysis

Empirical QA inspection of `inspection_result.json` revealed three primary issues:

1. **Unigram Keyword Fragmentation:** Proper nouns (`Def Jam Recordings`, `New York`) split into unigrams (`def`, `jam`, `new`, `york`).
2. **Concept Domain Leakage:** Unigram keyword fragments leak into `Metadata.Concepts` (`concept:york`).
3. **Inverted Relation Directionality:** Relation extraction asserts `Rick Rubin -> founded_by -> Def Jam` instead of `Def Jam -> founded_by -> Rick Rubin`.

---

## 3. Target Architecture

The target architecture decouples extraction algorithms behind **Pluggable Provider Interfaces**, enforcing strict domain boundaries:

```
                                  COMPOSITE ENRICHER PIPELINE
                                                │
       ┌────────────────────────────────────────┼────────────────────────────────────────┐
       ▼                                        ▼                                        ▼
┌──────────────┐                        ┌──────────────┐                        ┌──────────────┐
│  EntityPass  │                        │ ConceptPass  │                        │ RelationPass │
└──────┬───────┘                        └──────┬───────┘                        └──────┬───────┘
       │                                       │                                       │
       ▼ (EntityExtractor Seam)                ▼ (ConceptExtractor Seam)               ▼ (RelationExtractor Seam)
┌──────────────┐                        ┌──────────────┐                        ┌──────────────┐
│  Provider    │                        │  Filtered    │                        │ Validated    │
│  Strategy    │                        │  Concept     │                        │  SPO         │
└──────┬───────┘                        └──────┬───────┘                        └──────┬───────┘
       ├───────────────┬───────────────┐       └───────────────┐                       └───────────────┐
       ▼               ▼               ▼                       ▼                               ▼
┌─────────────┐ ┌─────────────┐ ┌─────────────┐         ┌─────────────┐                 ┌─────────────┐
│ RuleBased   │ │ GLiNER      │ │ Hybrid      │         │ Filtered    │                 │ Validated   │
│ Extractor   │ │ Client      │ │ Provider    │         │ Concept     │                 │ Relation    │
└─────────────┘ └─────────────┘ └─────────────┘         └─────────────┘                 └─────────────┘
```

---

## 4. Interface Contracts

### 4.1 EntityExtractor Interface
```go
type EntityInput struct {
	Chunks   []model.KnowledgeChunk
	Language string
	Labels   []string
}

type EntityExtractor interface {
	ExtractEntities(ctx context.Context, input EntityInput) ([]model.EntityMention, error)
}
```

### 4.2 ConceptExtractor Interface
```go
type ConceptInput struct {
	Tree     *model.SemanticTree
	Chunks   []model.KnowledgeChunk
	Keywords []model.Keyword
	Entities []model.Entity
	Language string
}

type ConceptExtractor interface {
	ExtractConcepts(ctx context.Context, input ConceptInput) ([]model.Concept, error)
}
```

### 4.3 RelationExtractor Interface
```go
type RelationInput struct {
	Chunks   []model.KnowledgeChunk
	Entities []model.Entity
	Concepts []model.Concept
}

type RelationExtractor interface {
	ExtractRelations(ctx context.Context, input RelationInput) ([]model.Relation, error)
}
```

---

## 5. Data Flow & Boundary Enforcement Rules

### Rule A: Entity Layer Boundaries
- Entities MUST represent concrete named objects (`Person`, `Organization`, `Location`, `WorkOfArt`, `Product`).
- Mentions are attached to `KnowledgeChunk.Entities` and aggregated into `DocumentMetadata.Entities`.

### Rule B: Concept Layer Boundaries
- Concepts MUST represent abstract document themes or structural headings (`concept:beginners-mind`).
- Concepts MUST NOT contain unigrams or single-word location/entity fragments (`concept:york`, `concept:def`).

### Rule C: Relation Layer Boundaries
- Relations MUST enforce predicate directional validity:
  - Correct: `organization:def-jam-recordings` -> `founded_by` -> `person:rick-rubin`
  - Correct: `person:rick-rubin` -> `founded` -> `organization:def-jam-recordings`
  - INVALID: `person:rick-rubin` -> `founded_by` -> `organization:def-jam-recordings`
- Relations connecting entities to unigram concept fragments MUST be filtered out.

---

## 6. Migration Strategy

1. **Phase 1: Seam Refactoring & Domain Filtering**
   - Refactor `EntityInput`, `ConceptInput`, `RelationInput` structs.
   - Add unigram concept filter to `RuleBasedConceptExtractor`.
   - Add predicate direction validator to `RuleBasedRelationExtractor`.
2. **Phase 2: Dependency Injection in `DefaultEnricher`**
   - Allow injecting custom `EntityExtractor`, `ConceptExtractor`, and `RelationExtractor` via constructor options.
3. **Phase 3: GLiNER Provider Client Prototype**
   - Implement `GLiNEREntityExtractor` HTTP REST client adapter with automatic fallback to `RuleBasedEntityExtractor`.
4. **Phase 4: Empirical Quality Validation**
   - Re-run quality inspection against `rick-rubin.pdf` and verify zero unigram concept leaks and zero predicate inversions.

---

## 7. Test Strategy

- **Unit Tests (`*_test.go`):** Verify `RuleBasedConceptExtractor` filters unigram fragments like `"york"`. Verify `RuleBasedRelationExtractor` produces `def-jam -> founded_by -> rick-rubin`.
- **Contract Tests:** Test `GLiNEREntityExtractor` client with mock HTTP server.
- **Integration Tests:** Test `DefaultEnricher` execution with custom injected providers.

---

## 8. Non-Goals

- Deploying Python GLiNER model to production Kubernetes/Docker environment.
- Implementing LLM-based abstractive extraction.
- Changing Knowledge Graph storage or indexing layer schemas.
