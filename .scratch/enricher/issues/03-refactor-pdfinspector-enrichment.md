# 03 — Refactor pdfinspector enrichment to CompositeEnricher Pass Architecture

**What to build:**
Refactor `DefaultEnricher` in `internal/pdfinspector/enrichment/enricher.go` to use `CompositeEnricher` with `TitleAuthorPass` and `PageResolutionPass`.

**Blocked by:** 02 — EnricherPass Pipeline Core.

**Status:** ready-for-agent

- [ ] Implement `TitleAuthorPass` using `TitleResolver` & `AuthorResolver` chains.
- [ ] Implement `PageResolutionPass` using `EnrichSemanticTree` page resolution.
- [ ] Update `DefaultEnricher` to delegate to `CompositeEnricher`.
- [ ] Add unit tests verifying end-to-end enrichment execution with zero hardcoded title fallbacks.
