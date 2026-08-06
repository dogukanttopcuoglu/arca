# M8 reranking destination & probe: second-stage reranking, measured approach selection

Status: accepted

M8 adds a second-stage reranking layer on top of the existing retrieval pipeline — nothing in today's retrieval behavior changes until benchmark evidence accepts a reranker. Reranking is ranked as a post-retrieval ordering correction layer, not as a replacement or mutation of any existing stage.

## Decisions

- **Destination.** Reranking is a second stage applied after retrieval: retrieve candidates (existing pipeline) -> rerank -> keep the caller's TopK. The existing retrieval pipeline (dense + sparse + graph fusion, orchestration gate, `FusionPolicy`) is not modified; rerank only reorders the candidate list.
- **Two-level success.** Primary success is measured on ranking metrics (nDCG@5, MRR@5). Secondary success is measured end-to-end on answer quality (EvidenceGate verification rate, citation behavior, correctness) — explicitly affected by downstream components, so it is secondary, never a substitute for the ranking measurement.
- **Approach selection by probe, not assumption.** An offline probe measures the single selected candidate before any implementation decision: cross-encoder (candidate: BGE-reranker-v2-m3). The probe measures the candidate's value against the production baseline — it does not validate a model in the abstract.
- **LLM rerankers excluded from M8.** LLM-as-reranker (e.g. RankGPT-style) is out of scope based on research evidence: seconds-scale latency per query, token cost, and no production vendor documentation of per-query LLM reranking (research report: `docs/research/rag-reranking-architectures.md`). Revisiting this requires a new decision.
- **Candidate budget sweep.** N = 20 / 50 / 100 (20: low-latency/low-cost bound; 50: common production point; 100: maximum reranker leverage). K is never swept — K is the caller's (AnswerEngine's) evidence budget, not a reranker concern. Default N is frozen by the benchmark result, never by intuition.
- **Probe scope discipline.** The probe is a measurement exercise, not an implementation. It measures quality delta, p50/p95 latency, memory footprint, and cold/warm model load time. "No candidate is good enough" is an explicit, valid outcome — see the closure rule below.
- **Closure rule.** "None accepted" is a success state of the probe, not a failure: if no approach passes the acceptance threshold (ADR-0045), M8 closes here, no production change is made, and the closure is recorded in an ADR. The probe's purpose is to decide by measurement, never to justify a predetermined solution. The model pool never auto-expands — adding a third candidate requires a new decision.

## Rationale

- The question M8 answers is "does adding a second-stage reranker to the production path produce value?" — that question is only measurable with the pipeline otherwise unchanged.
- Benchmark-gated discipline (M4, M7) carries forward: measurement decides, assumption does not; a negative result is a legitimate and expected possible outcome.

## Consequences

- No code changes to the retrieval pipeline, orchestration, or AnswerEngine until ADR-0045 acceptance.
- The probe result either freezes (model, N) and proceeds to ADR-0046 activation, or closes M8 with production byte-identical to today.
- Next: ADR-0044 (seam & execution contract), ADR-0045 (evaluation & kill gate), ADR-0046 (production activation), then `/to-spec`, `/to-tickets`, `/implement`.
