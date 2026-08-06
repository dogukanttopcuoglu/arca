# M7 graph benchmark dimension: four slices, acceptance rule, benchmark-gated

Status: accepted

M7's "does the graph signal work" question is settled by a dedicated benchmark dimension — a separate gold set over the same corpus, four query slices, and a formal accept/reject rule. Retrieval-level metrics are the acceptance surface; answer-level quality stays a separate evaluation track.

## Decisions

- **Four query slices:**
  - **Entity slice** (gain layer): entity-name queries; expected chunks are mention chunks — the formalized kill-gate slice.
  - **Entity-outside slice** (regression layer): single_fact / concept / procedural / comparison queries drawn from gold set v2 (untouched, referenced) — the graph stream must not damage dense retrieval here.
  - **Abstention slice:** the v2 abstention queries — the graph stream must not leak evidence into queries that should abstain.
  - **Frontmatter distinction** (honesty layer): entity-slice expected chunks are labeled substantive vs frontmatter; reported as mandatory diagnostics, never an acceptance criterion.
- **Comparison configs:** dense baseline / graph-only / dense+graph fusion (with the ticket-06 weights), all under the same retrieval contract: TopK 5, min-score 0.6.
- **Metrics:** primary recall@5 and MRR; supporting precision@5 and nDCG@5 (M3/M4/M5 comparability). Abstention metrics are reported separately (retrieved counts, no_evidence_precision, abstention recall/precision, generation-skipped delta) — the goal there is preventing evidence leakage, not finding hits.
- **Noise diagnostics (supporting):** per-query recall-regression counter (fusion recall < dense recall), graph-introduced chunks (dense-absent chunks added by fusion), intruder substantive/frontmatter ratio, and abstention leak count.
- **Acceptance rule (all three must hold):**
  1. Entity slice: `fusion recall@5 ≥ dense recall@5 × 1.05` and `fusion MRR ≥ dense MRR`.
  2. Entity-outside slice: fusion recall@5 and MRR within the 5% regression tolerance of dense (M3 rule).
  3. Abstention slice: the graph stream produces 0 chunks on abstention queries and the M5 gate metrics (no_evidence_precision, abstention recall/precision, generation-skipped) equal the baseline.
  - Failure of any criterion → milestone rejected; the system stays as-is (M7 success criteria #5).
- **Procedure:** the query set is a separate gold set file (v2 untouched, own corpus fingerprint, section-verified curation — the v2 method, not retrieval tuning). Eval-harness graph-stream wiring is introduced by the ticket-06 fusion decision.

## Rationale

- The rule makes "graph works" mean three things simultaneously: a real entity gain above noise (≥5%), no damage to the dense capability anywhere else (≤5% regression), and no leakage into abstention behavior — the formal counterpart of "not just entity recall increase, but no harm to dense".
- Frontmatter stays a diagnostic because retrieval gain and answer-level value must not be conflated; the label prepares the separate answer-level track without delaying the retrieval verdict.

## Consequences

- A new gold set file (schema 1.2 multi-doc, own fingerprint) is produced during build from this specification; gold set v2 and its fingerprint `8b21a664…` remain untouched.
- Ticket 06 (fusion path) is the remaining open decision; ticket 08 (orchestration gate) depends on 06 + this benchmark's outcome.
- **Gate-metric variance (errata 2026-08-05):** the M5 gate metrics (no_evidence_precision, abstention recall/precision, generation-skipped) come from LLM decisions and vary between runs — observed precision 0.500 vs 0.444 on identical retrieval. Retrieval metrics are deterministic. To stabilize the acceptance surface, `arc eval --gate-runs N` repeats each gate evaluation and records the median decision (retrieval runs once, retrieval metrics unchanged). Gate-metric acceptance should be read against this variance band; retrieval metrics remain the primary acceptance surface.
