# M7 Graph Fusion Calibration — Gold Set v3 Sweep

**Date:** 2026-08-05
**Protocol:** ADR-0040 acceptance rule on Gold Set v3 (37 queries: 8 entity / 21 entity-outside / 8 abstention; same corpus as v2, fingerprints `8b21a664…` + per-doc). Dense TopK 5, min-score 0.6, `--m5-gate`. Entity graph: 8 nodes persisted via the worker (`WithGraphStore`).
**Gate model:** DeepSeek `deepseek-v4-flash`.

## Results (per-slice recall@5 / MRR)

| config | entity recall@5 / MRR | outside recall@5 / MRR | abstention graph leak | gate precision |
|---|---|---|---|---|
| dense (baseline) | 0.276 / 0.458 | 1.000 / 0.897 | — | 0.471 |
| graph-only | 0.881 / 1.000 | 0.000 / 0.000 | **0/8** | 0.296 |
| fusion w0.5 | 0.298 / 0.625 | 1.000 / 0.873 | 0 (graph adds nothing) | 0.500 |
| **fusion w1.0 (frozen)** | **0.840 / 0.938** | 1.000 / 0.873 | **0 (graph adds nothing)** | **0.500** |
| fusion w2.0 | 0.881 / 1.000 | 1.000 / **0.833 (−7.1% MRR)** | 0 | 0.500 |

## Noise diagnostics (fusion w1.0 vs dense)

- **Recall-regression counter: 0** — no query regressed (fusion recall < dense recall).
- **Graph-introduced chunks: 19** (15 expected — the real contribution; 4 intruders, **3/4 frontmatter** — the documented honesty layer: most graph gain is real entity evidence, a small intruder share is frontmatter).
- Abstention: the graph stream produced **0 chunks on all 8 abstention queries** (graph-only run) and fusion added nothing to dense's retrieval on them — no leakage.
- M5 gate metrics: abstention precision 0.471 → 0.500, false abstentions 9 → 8, generation-skipped 17 → 16 — no regression (slight improvement).

## Acceptance verdict

- **fusion w1.0: PASS** — entity recall +204% (0.276 → 0.840) and MRR +105% (0.458 → 0.938) far above the ×1.05 rule; outside recall flat at 1.000 with MRR −2.7% (within 5%); abstention leak 0; gate metrics non-regressing.
- **fusion w2.0: FAIL** — outside MRR regresses −7.1%, beyond the 5% tolerance (ADR-0040 rule 2).
- **fusion w0.5: PASS** but lowest gain — not the freeze candidate.

## Decision

**GraphWeight freezes at 1.0** (highest-gain accepting point, regression-free). Production activation follows ADR-0042: `RETRIEVAL_GRAPH_WEIGHT=1.0`, entity intent + `UseGraph` gate, `WithGraphRetriever` engine injection. The eval flags remain the experiment surface for future re-calibration.

Reports: `m7_dense_v1.json`, `m7_graphonly_v1.json`, `m7_fusion_w05.json`, `m7_fusion_w10.json`, `m7_fusion_w20.json`. Gold set: `internal/eval/testdata/goldset_v3.json`.

## Gate-metric variance (M7 review BULGU-2, 2026-08-05)

The M5 gate metrics are LLM decisions and vary between runs: a repeat of the frozen w1.0 config reproduced the retrieval metrics byte-identically (entity 0.840/0.938, outside 1.000/0.873) but gate precision differed 0.500 → 0.444. Retrieval metrics are deterministic; gate metrics are not. Mitigation introduced: `arc eval --gate-runs N` repeats each gate evaluation (median decision wins) without touching retrieval metrics or the gate contract; acceptance should read gate metrics against this variance band, with retrieval metrics as the primary surface.
