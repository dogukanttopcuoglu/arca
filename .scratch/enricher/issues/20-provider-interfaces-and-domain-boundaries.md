# 20 — Provider Interfaces & Domain Boundaries

**Problem Statement:**
The strategy inputs `EntityInput`, `ConceptInput`, and `RelationInput` lack standardized fields for language, dynamic labels, and domain boundary filtering rules.

**Solution:**
Refactor `EntityInput`, `ConceptInput`, and `RelationInput` contracts in `entity_extractor.go`, `concept_extractor.go`, and `relation_extractor.go` to provide explicit provider seams and boundary context.

**Commits:**
1. `refactor(enrichment): standardize EntityInput, ConceptInput, RelationInput contracts`

**Blocked by:**
- None — can start immediately.

**Out of Scope:**
- GLiNER microservice implementation.
- Concrete extractor refactoring.
