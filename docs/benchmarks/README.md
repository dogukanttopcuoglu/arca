# ARC Retrieval Benchmarks

Benchmark reports produced by `arc eval` (ADR-0027). Every report is fully
reproducible: corpus fingerprint, embedding provider/model, retrieval mode,
TopK, MinScore, RRF parameters, collection, git commit, timestamp.

## How to run

```bash
set -a; source .env; set +a
./bin/arc.exe eval --mode dense --topk 5 --report docs/benchmarks/<name>.json
```

The runner hard-fails on corpus fingerprint mismatch before any query.

## Baseline — dense (top-5, no threshold)

File: `baseline_dense_v1.json` (commit 4626be4, corpus fingerprint
`51b1909e…`, 196 chunks, Ollama/nomic-embed-text:latest)

| Metric | Value |
|---|---|
| recall@5 | 0.740 |
| precision@5 | 0.288 |
| MRR | 0.666 |
| nDCG@5 | 0.645 |
| no_evidence_precision | 0.000 |

## Mode comparison — dense vs sparse vs hybrid (M3 T8)

Files: `baseline_dense_v1.json`, `baseline_sparse_v1.json`,
`baseline_hybrid_v1.json`. All three run on Gold Set v1 with the identical
corpus fingerprint (`51b1909e…`) and TopK 5.

| Metric | dense | sparse | hybrid |
|---|---|---|---|
| recall@5 | 0.740 | 0.682 | 0.734 |
| precision@5 | 0.288 | 0.247 | 0.279 |
| MRR | 0.666 | 0.585 | **0.736** |
| nDCG@5 | 0.645 | 0.577 | **0.668** |
| duration (51 queries) | ~10.8s | ~1.4s | ~3.8s |

Observations (decision data for T9, not conclusions):
- Sparse alone underperforms dense on recall/precision but is ~7.6x faster.
- Hybrid improves MRR (+10.5%) and nDCG@5 (+3.6%) over dense while recall@5
  (−0.8%) and precision@5 (−3.1%) stay within the 5% regression tolerance.
- Whether hybrid becomes the default retrieval mode is decided by T9 against
  these baselines — per-category breakdown and abstention behavior included.

Known baseline weaknesses (documented, not fixed in M3 baseline):
- Comparison queries (e.g. rr-028 "Everyone Is a Creator vs Beginner's Mind")
  retrieve one side only — recall 0; query decomposition is deferred (M4).
- Entity queries against low-frequency names (rr-038 Jackson Pollock, rr-042)
  miss at top-5 — the case for hybrid sparse retrieval (M3 T7–T9).
- `no_evidence_precision` is 0 by construction: with MinScore=0 the top-k is
  always returned, so abstention queries never abstain.

## RETRIEVAL_MIN_SCORE calibration (dense, top-5, Gold Set v1)

Full per-run data: `calibration_min_score_v1.json` (commit 7f139bcc).

| min_score | recall@5 | precision@5 | MRR | nDCG@5 | no_evidence_precision |
|---|---|---|---|---|---|
| 0.0 | 0.740 | 0.288 | 0.666 | 0.645 | 0.000 |
| 0.5 | 0.740 | 0.288 | 0.666 | 0.645 | 0.250 |
| **0.6** | **0.740** | **0.288** | **0.666** | **0.645** | **0.375** |
| 0.7 | 0.585 | 0.223 | 0.573 | 0.526 | 1.000 |

**Calibrated operating point: `RETRIEVAL_MIN_SCORE=0.6`** — the highest
threshold with recall@5 flat vs baseline (within the 5% regression budget)
while raising no_evidence_precision from 0 to 0.375. At 0.7 abstention
reaches 1.0 but recall regresses 21%, beyond the allowed budget.

**Conclusion (drives the spec, not the other way around):** a single global
cosine threshold cannot simultaneously maximize recall and abstention
precision on a semantically coherent corpus. High-precision abstention is
deferred to M4, where richer relevance signals (hybrid retrieval, sparse
evidence, reranking, semantic relevance gating) become available. M3
establishes the calibrated baseline only.

## Regression gates (calibration-first, per ADR-0027)

Thresholds are derived from this measured baseline — no invented numbers:

- **Tolerance:** a retrieval change must not regress any primary metric by
  more than 5% (relative) vs the committed baseline for the same mode.
- **Floors:** absolute floors at baseline − 5 points per primary metric
  (recall@5 ≥ 0.690, precision@5 ≥ 0.238, MRR ≥ 0.616, nDCG@5 ≥ 0.595).
- **Abstention:** a calibrated threshold must not lower recall@5 beyond the
  same tolerance on the non-abstention queries while raising
  no_evidence_precision.
- Every retrieval change (hybrid, RRF tuning, reranker, query decomposition,
  KG traversal) reports an `arc eval` delta vs the committed baseline for the
  same mode before merging. Improvement claims must be supported by the
  per-category metrics in the report.

## New baselines

When a retrieval mode or configuration is added (e.g. hybrid), record its
baseline in this directory with a full manifest before tuning begins.
