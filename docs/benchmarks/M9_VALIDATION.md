# M9 Document Overview — Validation

Status: accepted
Date: 2026-08-07
Gate: frozen criteria from the M9 spec — overview slice recall@5 ≥ 0.8 / MRR ≥ 0.9 (profile + first real content context jointly), v3.1 + E1 probe regression ≤ 5%, EvidenceGate active.

## What was validated

Document-level retrieval: each indexed document now carries a **Document Profile Point** (`content_type=document_profile`, deterministic content from enrichment outputs — title/author/extractive summary/key topics/sections), the rule-based analyzer detects the **document_overview** intent (capped EN+TR patterns), and the **Document Overview Retriever** serves it (profile + first real content chunks with a document filter; all profiles without one). Corpus fingerprint semantics unchanged — profiles are excluded (the fingerprint covers indexed *chunks*).

## Overview slice (Gold Set v5, 20 queries, EN+TR, 5 documents)

| metric | frozen threshold | measured | verdict |
|---|---|---|---|
| recall@5 | ≥ 0.8 | **1.000** | PASS |
| MRR | ≥ 0.9 | **1.000** | PASS |
| precision@5 | — | 1.000 | — |
| nDCG@5 | — | 1.000 | — |

Deterministic by construction: the retriever returns exactly the declared expectations (profile first, then the first real content chunks, `chunk_order > 1`). Evaluated on the profile + real content context jointly — not the profile alone.

## Regression gates

| gate | recorded baseline | re-run | Δ | verdict |
|---|---|---|---|---|
| E1 probe (entity-gated BGE) | ACCEPT, N=50: nDCG 0.897 (+1.20pp), MRR 0.902, verified 0.759, p95 3.97s | ACCEPT, N=50: 0.897 (+1.20pp), 0.902, 0.759, p95 4.02s | byte-identical | PASS |
| Gold Set v3.1 full eval (dense + graph w=1.0, min-score 0.6) | recall 0.956 / nDCG 0.886 / MRR 0.902 (probe baseline) | 0.956 / 0.879 / 0.891 | 0% / −0.8% / −1.2% | PASS (≤5%) |

The E1 probe re-run on the **unchanged** v3.1 artifact also re-verified the corpus fingerprint — profile points did not disturb it (ADR-0048 fingerprint decision).

## EvidenceGate

- Active and measured on the overview slice: DeepSeek gate, `gate_runs=3` (median), 19/20 contexts supported, 1 honest abstention (an overview context the gate judged unsupported — the gate is not bypassed on the new path).
- Manual E2E (production path, `-Doc rick`): "bu kitap ne anlatıyor", "yazarı kim", "ana fikri nedir" all return grounded, citation-backed answers (Document Profile + first real sections); "What does the book say about Rick Rubin?" still routes through the entity/graph path; unfiltered overview queries surface all profiles and the gate abstains honestly on "this book" ambiguity (Q4 decision — no new abstention semantics).

## Artifacts

- Reports: `.scratch/m9-document-overview/report_v5_overview.json` (overview slice + gate), `regression_e1_v31.json` (probe), `regression_v31_eval.json` (v3.1 eval).
- Gold set: `internal/eval/testdata/goldset_v5.json` (schema 1.2, `expected_retrieval_points` + per-query `document_id`).
- Decision record: ADR-0048 (flipped to accepted).
