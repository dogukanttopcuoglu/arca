# ADR-0025: Pluggable Enrichment Provider Architecture & GLiNER Integration Strategy

* **Status:** Accepted
* **Date:** 2026-08-02
* **Authors:** ARC Core Architecture Team
* **Deciders:** Staff Backend Engineer, Technical Lead

---

## 1. Context & Problem Statement

During runtime quality audits of the ARC Document Intelligence enrichment pipeline (`CompositeEnricher`), several extraction quality issues were observed on real PDF ingestion payloads:

1. **Unigram Keyword Fragmentation:** Proper nouns (`Def Jam Recordings`, `New York`) were split into unigram keyword fragments (`def`, `jam`, `new`, `york`).
2. **Boundary Leakage:** Keyword fragments leaked into the `Metadata.Concepts` model (e.g., `concept:york`), polluting concept taxonomy with location fragments instead of abstract document themes.
3. **Inverted Relation Directionality:** Relation extraction produced inverted Subject-Predicate-Object triples (e.g., `Rick Rubin -> founded_by -> Def Jam` asserting that Rick Rubin was founded by Def Jam).
4. **Low Entity Recall:** The initial `RuleBasedEntityExtractor` achieved high precision on seeded patterns but failed to recognize un-seeded entity mentions (e.g., `Russell Simmons`).

Hardcoding extraction algorithms directly into pipeline passes limits evolution. Changing the extraction model (e.g., upgrading from rule-based regex to zero-shot GLiNER or LLMs) should not require altering pipeline orchestration or data contracts.

---

## 2. Decision Outcome

We decision to evolve the ARC enrichment layer into a **Pluggable Provider Architecture** based on Strategy Seams and Dependency Injection.

### Key Decisions:

1. **Decoupled Extraction Seams:**
   Declare explicit strategy interfaces for the three primary enrichment responsibilities:
   - `EntityExtractor`
   - `ConceptExtractor`
   - `RelationExtractor`

2. **Domain Boundary Rules:**
   Enforce strict isolation between model layers:
   - **Entity:** Concrete named mentions (Person, Organization, Location, Product, WorkOfArt).
   - **Concept:** Abstract document themes or structural headings. MUST NOT contain single-word unigrams or location/entity fragments (`york`, `def`).
   - **Relation:** Directed SPO triples with validated predicate directionality (`Def Jam -> founded_by -> Rick Rubin`).

3. **Pluggable Engine Providers:**
   Concrete extraction strategies (e.g., `RuleBasedEntityExtractor`, `GLiNEREntityExtractor`, `LLMEntityExtractor`, `HybridEntityExtractor`) implement these seams. GLiNER is selected as the first production candidate provider behind `EntityExtractor`.

4. **CompositeEnricher Dependency Injection:**
   `CompositeEnricher` receives extractor strategy implementations via dependency injection. Pipeline orchestration (`LanguageDetectionPass` -> `TitleAuthorPass` -> `KeywordExtractorPass` -> `EntityExtractorPass` -> `ConceptExtractorPass` -> `RelationExtractorPass` -> `SummaryPass`) remains 100% frozen.

---

## 3. Scope Boundaries

### In Scope:
- `EntityExtractor`, `ConceptExtractor`, `RelationExtractor` Go strategy interfaces.
- Migration of existing rule-based extractors behind these interfaces.
- Dependency injection in `CompositeEnricher` / `DefaultEnricher`.
- Strict Entity ➔ Concept ➔ Relation domain boundary filtering rules.
- Relation predicate direction validation rules.

### Out of Scope:
- Full Python GLiNER microservice production deployment infrastructure.
- LLM-based abstractive extraction passes.
- Knowledge Graph database/storage changes (`internal/graph`).

---

## 4. Consequences & Benefits

- **High Testability:** Strategy implementations can be mocked or unit-tested in isolation.
- **Zero Pipeline Churn:** Upgrading an extraction engine (e.g. Rule-Based ➔ GLiNER ➔ LLM) requires zero changes to pipeline passes or orchestrators.
- **Empirical Quality Improvement:** Predicate direction validation and concept boundary filtering eliminate relation and concept noise.
