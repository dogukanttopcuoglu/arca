# 02 — EnricherPass Interface & CompositeEnricher Pipeline Runner

**What to build:**
Implement `EnricherPass` interface, `Capability` enum, and `CompositeEnricher` pipeline runner in `internal/pdfinspector/enrichment/pass.go`.

**Blocked by:** 01 — Title & Author Resolver Chains.

**Status:** ready-for-agent

- [ ] Define `Capability` enum and `EnricherPass` interface seam (`Name`, `Requires`, `Provides`, `Execute`).
- [ ] Implement `CompositeEnricher` executing an ordered sequence of passes.
- [ ] Add capability contract validation before executing passes.
- [ ] Add unit tests verifying pass ordering and capability validation error handling.
