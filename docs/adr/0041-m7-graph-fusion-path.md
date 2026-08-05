# M7 graph fusion path: separate composition retriever, benchmark-calibrated weight

Status: accepted

The graph stream enters the retrieval pipeline as an additive retrieval source through a new `seam.Retriever` composition — not by extending the frozen M4 fusion machinery. Its weight is chosen by the ADR-0040 benchmark through a small, interpretable sweep; production behavior stays dense-only until a sweep point passes.

## Decisions

- **Separate composition retriever.** `GraphFusionRetriever(denseRetriever, graphRetriever, config)` implements `seam.Retriever` and fuses the dense and graph streams with RRF. `HybridRetriever` keeps its dense+sparse responsibility; `FusionPolicy` (ADR-0029) is untouched. Dense-only behavior is preserved as a regression guard: `GraphWeight = 0` returns the dense stream unchanged.
- **Sparse stays out of v1 fusion.** Three-way dense+sparse+graph fusion requires its own benchmark evidence and a wider two-dimensional sweep; it is not introduced now.
- **Independent weight surface.** `GraphFusionConfig{DenseWeight, GraphWeight}` mirrors the shape of `FusionPolicy` but is independent. No named presets — names freeze only after calibration (M4 lesson). A config-mutation seam exists for eval sweeps (the `HybridRetriever.SetFusionPolicy` pattern). `RRFK` is a retriever constant (60, frozen M4 parameter), deliberately outside the config: the sweep covers only the weights. Graph-only measurement is not a mode of the fusion retriever (RRF maps non-positive weights to 1.0) — `--graph-only` runs use the graph retriever directly.
- **Eval surface is additive.** `arc eval --graph-weight <w>` (default 0 → today's behavior byte-identical) and `--graph-only` (measures the graph stream alone, the kill-gate graphA counterpart). The `RetrievalMode` enum is not extended — graph fusion is a composition option, not a mode. The report manifest records `graph_weight`, `graph_only`, and the fusion config (ADR-0027 reproducibility).
- **Calibration protocol.** Sweep points `0.5 / 1.0 / 2.0`; each measured against the ADR-0040 acceptance rule (entity slice: fusion recall ≥ dense × 1.05 and MRR ≥ dense; entity-outside: ≤5% regression; abstention: zero leak and preserved M5 gate metrics). Freeze: among accepting points, the highest-gain regression-free value becomes the `GraphFusionConfig` default — a single frozen value, no presets. Rejection: no point accepts → production default stays 0, the eval flag remains as an experiment surface, the graph code stays (rejection means not entering the production path, not deleting code), and the milestone closes with a documented rejection.

## Rationale

- The path preserves every frozen contract (M4 `FusionPolicy`, M6 `RetrievalDecision`/`IntentHint`, gold set v2) while giving the graph signal a first-class, measurable entry point; the eval harness measures at the retriever seam, so ADR-0040's dense/graph/fusion comparison runs on the same surface.
- Hardcoding is replaced by measurement: the weight enters configuration only after a benchmark verdict; until then `GraphWeight = 0` keeps production behavior unchanged.

## Consequences

- Build work (after the map clears, via `/to-spec` → `/to-tickets` → `/implement`): `GraphFusionConfig`, `GraphFusionRetriever`, runtime graph-store wiring (`QdrantGraphStore`), eval flags and manifest fields, then the three-point sweep against the ADR-0040 benchmark and the freeze/reject decision.
- Ticket 08 (orchestration gate) is the last open map decision; it depends on this path and the benchmark outcome.
