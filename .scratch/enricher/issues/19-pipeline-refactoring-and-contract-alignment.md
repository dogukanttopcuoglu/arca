# 19 — Pipeline Refactoring & Contract Alignment

**Problem Statement:**
The enrichment pipeline contains legacy hardcoded fallbacks ("The Creative Act: A Way of Being", "Rick Rubin") in `enricher.go` that violate ADR-0019, duplicated title/author resolution functions, and a capability contract mismatch in `EntityExtractorPass.Requires()` (missing `CapabilitySemanticTree`).

**Solution:**
Remove legacy hardcoded fallbacks from `enricher.go`, delegate metadata resolution exclusively to `resolver.go`, align `EntityExtractorPass` capability contract with ADR-0021, and apply normalized string matching for entity grouping and concept tagging.

**Commits:**
1. `refactor(enrichment): remove legacy hardcoded title/author fallbacks from enricher.go`
2. `refactor(enrichment): eliminate duplicated title and author resolution logic`
3. `fix(enrichment): align EntityExtractorPass capability contract with ADR-0021`
4. `refactor(enrichment): normalize entity grouping in EntityExtractorPass`
5. `refactor(enrichment): use normalized matching in ConceptExtractorPass`

**Decision Document:**
- Single source of truth for metadata resolution is `resolver.go`.
- `EntityExtractorPass` explicitly requires `CapabilitySemanticTree`.
- Entity grouping uses normalized `strings.ToLower(m.Text)`.

**Testing Decisions:**
- Run `go test ./internal/pdfinspector/enrichment/...` after each commit step.

**Out of Scope:**
- $O(N \cdot M \cdot C)$ relation loop optimization (deferred until benchmarked).
- TF-IDF allocation optimizations (deferred until benchmarked).
- Advanced sentence splitting (acceptable MVP limitation).
