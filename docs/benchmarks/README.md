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

Known baseline weaknesses (documented, not fixed in M3 baseline):
- Comparison queries (e.g. rr-028 "Everyone Is a Creator vs Beginner's Mind")
  retrieve one side only — recall 0; query decomposition is deferred (M4).
- Entity queries against low-frequency names (rr-038 Jackson Pollock, rr-042)
  miss at top-5 — the case for hybrid sparse retrieval (M3 T7–T9).
- `no_evidence_precision` is 0 by construction: with MinScore=0 the top-k is
  always returned, so abstention queries never abstain. A calibrated
  `RETRIEVAL_MIN_SCORE` (T5) is expected to fix this; the value is set from
  measurement, not invented.

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
