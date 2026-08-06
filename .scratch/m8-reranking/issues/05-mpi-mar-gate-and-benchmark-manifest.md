# 05 — MPI/MAR gate and benchmark manifest

**What to build:** The kill gate (ADR-0045) evaluated automatically against frozen, baseline-relative thresholds: MPI (minimum practical improvement) nDCG@5 delta >= +1pp; MAR (maximum acceptable regression) MRR delta >= -0.5pp and verified-rate delta >= -1pp; abstention behavior identical (hard invariant, no tolerance); operational budgets (p95 latency, memory) frozen in the benchmark manifest before the probe starts. Bootstrap CI is reported but is never a decision criterion. Deterministic selection rule: highest quality among accepted combinations wins; within 5% tolerance, smallest N wins.

**Blocked by:** 04 — Probe simulation runs and metrics.

**Status:** resolved

- [ ] Gate evaluates all thresholds automatically (MPI, MAR, abstention hard invariant)
- [ ] Operational budgets (p95 latency, memory) frozen in the manifest before the probe run
- [ ] Bootstrap CI reported as informational only, never a decision input
- [ ] Deterministic selection rule implemented (highest quality; smallest N within tolerance)
- [ ] "None accepted" is a first-class gate outcome (no forced winner)

## Comments
