# 23 — GLiNER Entity Provider Client Adapter Prototype

**Problem Statement:**
High-recall zero-shot entity extraction requires an HTTP REST adapter client (`GLiNEREntityExtractor`) capable of communicating with a GLiNER NER microservice with automatic fallback to `RuleBasedEntityExtractor`.

**Solution:**
Implement `GLiNEREntityExtractor` in `internal/pdfinspector/enrichment/gliner_client.go` implementing `EntityExtractor`, forwarding chunk text and dynamic entity labels to an external endpoint, and falling back gracefully on network error.

**Commits:**
1. `feat(enrichment): implement GLiNEREntityExtractor HTTP client adapter with fallback`

**Blocked by:**
- #20 (Provider Interfaces & Domain Boundaries)
- #22 (Composite Enricher Dependency Injection)

**Out of Scope:**
- Python service deployment.
