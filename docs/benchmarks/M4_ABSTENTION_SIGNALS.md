# M4 Abstention Signals — Measurement Report

**Date:** 2026-08-03
**Protocol:** measure two deterministic retrieval-tier signals over Gold Set v1.1
(8 abstention + 47 relevance queries, dense mode, TopK 5); replace the
calibrated `RETRIEVAL_MIN_SCORE=0.6` threshold only if no_evidence_precision
≥ 0.9 with recall degradation ≤ 5%. Data: `v11_signals_dense.json`.

## Signal 1 — Distinctive-term lexical coverage

Query terms with corpus DF ≤ 3, fraction found in retrieved chunk content.

| class | coverage = 0 | coverage ≥ 0.5 |
|---|---|---|
| abstention (8) | 7 | 1 (rr-046 Def Jam, 0.4) |
| relevance (47) | **15** | 32 |

Coverage does not discriminate: dense retrieval frequently returns
semantically-similar chunks whose *distinctive exact terms* differ (15 real
queries at coverage 0), so a coverage rule that catches abstention queries
also false-abstains a third of the relevance set.

## Signal 2 — Top-1/top-2 score gap

| class | gap range |
|---|---|
| abstention (8) | 1.00 – 1.15 |
| relevance (47) | 1.00 – 1.29 |

Complete overlap. Dense top-5 cosine scores cluster tightly (ratios just
above 1.0) for both classes; "a clear winner" is not a property of grounded
queries on this corpus.

## Operating-point scan

Every threshold combination either missed abstention queries or
false-abstained relevance queries; no rule reaches no_evidence_precision
≥ 0.9 with recall ≥ 95% of baseline. The T5 finding is confirmed at the
signal level: **retrieval-tier abstention has reached its deterministic
ceiling on this corpus** — nonsense queries embed inside the book's semantic
cluster and share its score profile.

## Decision

- **Keep `RETRIEVAL_MIN_SCORE=0.6`** (calibrated M3 operating point,
  abstention precision 0.375, recall flat). No replacement.
- The signals remain in the benchmark as recorded diagnostics
  (`abstention_signals` per query) for future mechanisms to compare against.
- **Semantic abstention is M5's problem**, where the generation tier answers
  "do the sources actually answer this?" with full context. The mechanism is
  not a retrieval-tier failure; it is a retrieval-tier ceiling, measured.

This is the benchmark-driven conclusion the protocol specified: the
conclusion is not "failure" — it is the deterministic ceiling, quantified.
