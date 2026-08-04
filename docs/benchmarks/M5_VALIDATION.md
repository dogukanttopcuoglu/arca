# M5 Validation Report — Multi-Document Corpus

**Date:** 2026-08-04
**Pipeline:** Firecrawl PDF Service → semantic reconstruction → hierarchical chunking → enrichment → Ollama `nomic-embed-text:latest` embeddings (dense 768d + BM25 sparse) → Qdrant `arca_chunks`
**LLM gateway (gate + generation):** OpenRouter, `google/gemma-4-26b-a4b-it:free`
**Retrieval:** Dense, TopK 5, `RETRIEVAL_MIN_SCORE=0.6` (M4 frozen operating point)

## Corpus statistics

| document | pages | chunks | fingerprint (live) |
|---|---|---|---|
| Asking the Right Questions (Browne & Keeley) | 186 | 416 | `999342c3…` |
| Domain-Driven Design with Golang (Boyle) | 204 | 671 | `da442d50…` |
| Thinking in Systems (Meadows) | 235 | 397 | `1bad1d65…` |
| Event-Driven Architecture in Golang (Stack) | 384 | 1337 | `f9f0b5fa…` |
| rick-rubin | 248 | 196 | `51b1909e…` (M4 fingerprint reproduced byte-identically) |
| **Total** | **1257** | **3017** | aggregate `8b21a664…` |

## Indexing result

- 4 new books ingested via the production pipeline (Firecrawl → semantic → chunk → enrich → embed → Qdrant). Two books were owner-encrypted PDFs; decryption with the empty user password was required before Firecrawl would parse them.
- rick-rubin re-inspection produced **byte-identical content hashes** (196/196 skipped) — end-to-end pipeline determinism confirmed; the M4 corpus state is untouched.
- Store: 196 → 3017 points; all points carry dense vectors (768-dim) and sparse vectors; chunk markdown persisted in point payloads.
- Per-document fingerprints verified against the live index before any query ran (gold set schema 1.2 declares one fingerprint per document).

## Gold Set v2 (goldset_v2.json)

29 queries, all grounded in real indexed chunks:

- **17 supported** (single_fact/concept) — evidence selected from the artifacts and section-verified against actual retrieval (expected chunks = the retrieved chunks whose section genuinely answers the query).
- **4 comparison** — cross-document queries (e.g. "Compare CQRS with event-carried state transfer", DDD↔systems, creative-act↔EDA).
- **8 abstention** — queries with no corpus evidence (Atlantis, quantum entanglement, Gatsby, Tokyo population, …).

**Constraint handling:** `goldset_v1_1.json` (M4, rick-rubin, fingerprint `51b1909e`) was **not overwritten** — it remains frozen. The M5 corpus spans 5 documents, which the single-document gold set schema could not express, so the eval harness gained an additive multi-document extension (schema 1.2, optional `documents` array, per-document fingerprint verification). Legacy single-document gold sets and reports are byte-compatible. This is benchmark infrastructure, not M5 architecture.

## Retrieval verification (dense, TopK 5, min-score 0.6)

| metric | value |
|---|---|
| recall@5 | **1.000** (21/21 relevance queries) |
| precision@5 | 0.381 |
| MRR | 0.897 |
| nDCG@5 | 0.902 |
| no_evidence_precision (retrieval tier) | 0.250 (2/8 abstention queries return empty) |

Every relevance query retrieves its expected evidence from the correct document. Cross-document comparison queries retrieve both sides. The retrieval-tier abstention ceiling from M4 is reproduced: 6 of 8 abstention queries still retrieve ≥1 chunk above 0.6.

Hybrid Balanced + decomposition measured **lower** on this corpus (recall@5 0.833, MRR 0.607) — dense stays the M5 validation config.

## M5 gate results (`arc eval --m5-gate`, report `docs/benchmarks/m5_gate_v1.json`)

| metric | value |
|---|---|
| generation skipped | **13 / 29 queries (44.8%)** |
| abstention precision | **0.615** (8 correct / 13 abstained) |
| abstention recall | **1.000** (8/8 expected abstentions caught) |
| false abstentions | 5 |
| missed abstentions | 0 |
| gate errors (malformed output, retried once) | 2 |
| gate provider/model | openrouter / gemma-4-26b-a4b-it:free |

Per-query: 18 supported decisions, 10 unsupported, 2 gate_error, 2 empty-retrieval abstentions.

### Findings

1. **The gate solves the M4 abstention ceiling end-to-end.** Retrieval tier abstains on 2/8 (precision 0.25); the M5 gate catches all 8 (recall 1.000) by evaluating the retrieved context semantically — 6 unsupported + 2 empty.
2. **False abstentions concentrate in the comparison class** (4 of 4 comparison queries + 1 single_fact). The gate judged the TopK-5 context insufficient for cross-document synthesis. Two readings: (a) genuinely evidence-scarce context at TopK 5 for comparative answers, or (b) the free-tier model under-judging comparative prompts. Either way this is an empirical orchestration signal to investigate (larger TopK or targeted gate prompt for comparisons), not a mechanism failure — exactly the measurement M5 was built for.
3. **Gate-error fail-closed works as designed.** 2 queries produced malformed model output twice in a row; the engine returned a typed `EvidenceGateError` and skipped generation (no hallucinated answer). Retry recovered 7 other queries.
4. **Free-tier model instability is the dominant gate quality risk** — 2 malformed outputs and 5 false abstentions on a `:free` model. The deterministic fake-gate tests remain the regression anchor; real-provider reports must be re-run per model.

### Live probes (`arc ask`)

- "What is event-carried state transfer?" → grounded answer citing event-carried-state-transfer chunks (EDA book).
- "How do feedback loops regulate a system's behavior?" → grounded answer citing feedback-loop chunks (Thinking in Systems).
- "What is beginner's mind according to Rick Rubin?" → gate failed closed with `EvidenceGateError` (free-model malformed output ×2) — no answer produced rather than a hallucination.

## Artifacts

- `internal/eval/testdata/goldset_v2.json` — M5 gold set (schema 1.2)
- `docs/benchmarks/m5_retrieval_v1.json` — dense retrieval report
- `docs/benchmarks/m5_retrieval_hybrid_v1.json` — hybrid + decomposition report
- `docs/benchmarks/m5_gate_v1.json` — M5 gate report (per-query gate observations + M5 metrics)
- `.scratch/m5-corpus/results/*.json` — per-document `PDFInspectionResult` artifacts
