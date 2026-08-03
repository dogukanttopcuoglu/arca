# M4 Fusion Ceiling Experiment — Sweep Summary

**Date:** 2026-08-03
**Question:** Can intent-based retrieval quality be meaningfully improved by changing only static fusion policies over the existing Dense + Sparse stack?
**Protocol:** 11 configurations, Gold Set v1, fingerprint `51b1909e…`, TopK 5, frozen baselines. Per-intent metrics are the decision metrics.

## Results

| config | recall@5 | MRR | nDCG@5 | entity rec | single_fact rec/mrr | comp rec | proc rec |
|---|---|---|---|---|---|---|---|
| baseline (M3 frozen) | 0.734 | 0.736 | 0.668 | 0.875 | 0.963 / 0.944 | 0.521 | 0.615 |
| λs=1.0 (=baseline) | 0.734 | 0.736 | 0.668 | 0.875 | 0.963 / 0.944 | 0.521 | 0.615 |
| λs=0.75 | 0.740 | 0.736 | 0.689 | **0.750** | 0.963 / 0.889 | **0.646** | 0.625 |
| λs=0.5 | 0.740 | 0.713 | 0.673 | **0.750** | 0.963 / 0.833 | **0.646** | 0.625 |
| λs=0.25 | 0.740 | 0.717 | 0.675 | **0.750** | 0.963 / 0.833 | **0.646** | 0.625 |
| cap=3 | 0.740 | 0.686 | 0.640 | 0.875 | 0.963 / 0.889 | 0.458 | 0.583 |
| cap=5 / cap=10 / unlimited | 0.734 | 0.736 | 0.668 | 0.875 | 0.963 / 0.944 | 0.521 | 0.615 |
| λs=0.5 + cap=3 | 0.740 | 0.728 | 0.687 | **0.750** | 0.963 / 0.889 | **0.646** | 0.625 |
| λs=0.75 + cap=3 | 0.740 | 0.728 | 0.687 | **0.750** | 0.963 / 0.889 | **0.646** | 0.625 |

Manifest check: `fusion_policy: {"dense_weight": 1, "sparse_weight": 0.5, "sparse_cap": 3, "rrf_k": 60}` — recorded per artifact.

## Findings

1. **Default-policy gate passed:** the M3 hybrid baseline reproduces byte-identically under the FusionPolicy mechanism (0.734 / 0.736 / 0.668). The change is additive; no behavior drift.
2. **Cap is dominated:** cap=3 hurts ranking (MRR −0.050, nDCG −0.028); cap ≥5 has zero effect (sparse streams return exactly TopK=5 at this depth). The cap mechanism adds nothing at TopK=5 — rejected.
3. **Weight reduction recovers the M3 regressions:** λs≤0.75 recovers comparison recall 0.521 → **0.646** (+0.125, the biggest single gain in the sweep) and procedural 0.615 → 0.625.
4. **Weight reduction destroys the M3 wins:** entity recall 0.875 → **0.750** (−14.3%) at every λs<1.0 — the exact-token rescue is weight-sensitive and collapses. Single_fact ranking MRR also drops 0.944 → 0.833–0.889.
5. **No uniform policy passes the acceptance rule.** The rule required procedural+comparison recovery AND entity+single_fact within 5% — any λs<1.0 recovers the first pair but violates the second by 3× the tolerance. λs=1.0 keeps the wins but leaves the regressions.

## Conclusion

**Uniform fusion recalibration is rejected by the benchmark.** But the ceiling experiment succeeded in its primary purpose: it proves the intent-aware ceiling is large (comparison +0.125 recall) and *only* reachable by per-intent policy selection — a single static policy cannot capture both sides of the trade-off. This is the empirical mandate for the M5 orchestrator.

## M4 artifacts frozen from this data

- **Balanced** `{λd:1.0, λs:1.0, cap:0, k:60}` — serves entity and single_fact; this is the M3 default and remains the M4 default.
- **DenseBiased** `{λd:1.0, λs:0.5, cap:0, k:60}` — serves comparison and procedural (ceiling recovery at λs=0.5; cap=3 adds nothing).
- **LexicalBiased** — no calibration data supports a sparse-biased policy on this corpus; left undefined until evidence exists (no λs>1.0 sweep showed value).
- Weighted RRF and the cap mechanism stay in the code behind `FusionPolicy`; the orchestrator (M5) selects among the frozen policies via IntentHint.

The M4→M5 handoff is now quantified: the orchestrator's job is `Comparison/Procedural → DenseBiased`, `Entity/SingleFact → Balanced`, with a measured ceiling of +0.125 comparison recall and a known −0.125 entity cost of getting the selection wrong.
