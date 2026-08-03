# M4 retrieval intelligence: calibrated fusion policies, rule-based decomposition, and the abstention ceiling

Status: accepted

M4 calibrated the existing retrieval stack — no new retrieval paradigms. Weighted-RRF and sparse-candidate-cap mechanisms were implemented behind a frozen `FusionPolicy` type, calibrated by an 11-config sweep, and named policies (`Balanced`, `DenseBiased`) became the only fusion variants. Rule-based query decomposition was added for comparison queries with a shared merge seam. Retrieval-tier abstention signals were measured and found to have a deterministic ceiling; semantic abstention defers to M5.

## Decisions

- **Fusion calibration (sweep, 11 configs on Gold Set v1, fingerprint `51b1909e…`):** the default policy reproduces the M3 hybrid baseline byte-identically (backward-compatibility contract). Uniform weight reduction recovers comparison recall (+0.125) but destroys the entity exact-token rescue (−14.3% recall) — **no uniform policy passes the acceptance rule**. The intent-aware ceiling is therefore large and only reachable by per-intent selection, which is M5's mandate.
- **Named policies (M4 static orchestration only):** `Balanced` (default, M3-compatible) and `DenseBiased` (λs=0.5) are frozen; `LexicalBiased` remains undefined until data supports it. Configuration selects policies (`RETRIEVAL_FUSION_POLICY`); `arc eval --fusion-policy densebiased` reproduces the sweep numbers exactly — machinery verified independently of any future intent routing. The sparse cap mechanism is retained but rejected at TopK=5 (cap≥5 no effect; cap=3 hurts ranking).
- **Rule-based decomposition (accepted by benchmark):** comparison patterns split into deterministic sub-queries in the `QueryAnalyzer`; the engine and the benchmark share `MergeRankedLists` (rank-interleave, ChunkID dedupe, TopK contract). On hardened Gold Set v1.1 (four non-patterned comparison forms), hybrid Balanced + decomposition beats the DenseBiased ceiling: comparison recall +0.104, entity +0.125, single_fact 0.000, concept −2.4%, procedural −1.6% — all within tolerance. Decomposition survives M4.
- **Abstention ceiling (measured):** distinctive-term lexical coverage and top-1/top-2 score gap do not discriminate abstention from relevance queries on this corpus (15 of 47 relevance queries at coverage 0; score-gap distributions overlap). No deterministic rule reaches no_evidence_precision ≥ 0.9 within the recall tolerance. `RETRIEVAL_MIN_SCORE=0.6` stays frozen; the signals remain recorded diagnostics.
- **M4/M5 boundary:** M4 owns calibration and static machinery (policies, decomposition trigger, measurement). M5 owns orchestration: `IntentHint → named policy` selection and generation-tier semantic abstention ("do the sources actually answer this?"). No runtime intelligence entered M4; the decision surface is intentionally empty.

## Rationale

- The benchmark proved each decision: the fusion sweep rejected uniform recalibration, the decomposition experiment accepted the rule-based trigger against the calibrated ceiling, and the abstention protocol quantified the deterministic ceiling the protocol specified. Every decision is reproducible from committed reports and the frozen fingerprints.
- Keeping numerical optimization inside M4 and selection inside M5 preserves causality: M5's gains are measured against frozen, named policies.

## Consequences

- `arc ask` keeps dense mode and Balanced policy by default; hybrid + decomposition is the recommended configuration pending M5 orchestration.
- Future work (rerankers, KG, agentic) measures against the M3 baselines plus the M4 artifacts: `fusion_sweep/summary.md`, `M4_ABSTENTION_SIGNALS.md`, and Gold Set v1.1.
- The abstention-signal diagnostics and the shared merge seam are permanent benchmark infrastructure.
