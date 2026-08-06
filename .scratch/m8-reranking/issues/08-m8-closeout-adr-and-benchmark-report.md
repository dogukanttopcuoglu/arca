# 08 — M8 closeout: ADR and benchmark report

**What to build:** The M8 closeout documentation. On acceptance: the benchmark report (metrics, CI, gate outcome, artifacts) committed to the benchmarks docs, the frozen (model, N) values recorded, and the ADR-0046 activation decisions annotated with the actual calibrated values. On rejection: a closure ADR recording "none accepted" as the outcome, production byte-identical confirmation, and the model pool rationale (no auto-expansion). Either path records the probe's evidence so the decision is auditable.

**Blocked by:** 06 — Probe run and freeze/closure decision.

**Status:** ready-for-agent

- [ ] Benchmark report committed (metrics per combination, CI report-only, gate outcome)
- [ ] Acceptance: frozen (model, N) recorded against the artifact fingerprint
- [ ] Rejection: closure ADR written; production byte-identical confirmed
- [ ] ADR-0043...0046 annotated/closed with the actual outcome
- [ ] Evidence trail complete: artifacts + manifest + report auditable

## Comments
