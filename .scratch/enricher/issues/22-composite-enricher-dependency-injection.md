# 22 — Composite Enricher Dependency Injection

**Problem Statement:**
`DefaultEnricher` constructs extractor passes using default rule-based implementations, preventing injection of custom provider strategy instances (e.g. GLiNER or Hybrid extractors).

**Solution:**
Add functional options or constructor parameters to `DefaultEnricher` and `NewEnricherWithExtractors` to support dependency injection for `EntityExtractor`, `ConceptExtractor`, and `RelationExtractor`.

**Commits:**
1. `refactor(enrichment): support dependency injection in DefaultEnricher`

**Blocked by:**
- #20 (Provider Interfaces & Domain Boundaries)

**Out of Scope:**
- External service deployment.
