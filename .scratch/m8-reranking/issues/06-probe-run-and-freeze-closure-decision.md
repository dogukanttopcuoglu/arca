# 06 — Probe run and freeze/closure decision

**What to build:** The actual benchmark run: execute the probe on Gold Set v3 against the corpus fingerprint, produce the full report (metrics + gate outcome), and make the decision. Acceptance freezes (model, N) as the benchmark-accepted configuration; rejection produces the "none accepted" outcome. Either way production behavior is byte-identical to today until ticket 07 executes — the decision itself changes nothing in code.

**Blocked by:** 04 — Probe simulation runs and metrics; 05 — MPI/MAR gate and benchmark manifest.

**Status:** ready-for-agent

- [ ] Probe executes end-to-end with the frozen manifest (fingerprint, gold set v3, budgets)
- [ ] Report records every combination's metrics, CI, and gate outcome
- [ ] Decision is deterministic: accepted (model, N) frozen, or "none accepted"
- [ ] No production code changes result from this ticket (decision-only)
- [ ] Artifacts and manifest committed as the benchmark's evidence trail

## Comments
