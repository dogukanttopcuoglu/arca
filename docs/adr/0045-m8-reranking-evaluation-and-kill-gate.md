# M8 reranking evaluation & kill gate: delta isolation, candidate artifacts, frozen acceptance thresholds

Status: accepted

The probe measures only the reranker's contribution: baseline and experiment differ in exactly one variable — the `RerankedRetriever` wrapper with candidate budget N. Acceptance thresholds are frozen before the benchmark starts; they are baseline-relative, never absolute metric targets.

## Decisions

- **Delta isolation.** Baseline: `GraphFusionRetriever` (M7 production config, w=1.0, K=5). Experiment: the same `GraphFusionRetriever`, same graph weight, same corpus, same gold set (v3), same AnswerEngine, same EvidenceGate — the only difference is the wrapper and N. The measured delta therefore represents the reranker's contribution only.
- **Deterministic candidate generation.** Every combination runs on the same corpus fingerprint, same embeddings, same graph index, and the same candidate set. Candidate lists are recorded as benchmark artifacts so different rerankers can be re-run over the identical candidates later and retrieval variance is fully eliminated. **Candidate artifact format (normative):** benchmark fingerprint; query id; ordered candidate ids; baseline retrieval metadata; candidate scores (informational only, never a decision input); reranker output ordering.
- **Metrics (every combination, 2 models x 3 N = 6 + baseline).** nDCG@5 and MRR with binary relevance from gold set v3's `expected_chunk_ids` (graded relevance does not exist; IDCG computed from the ideal ordering of relevant items, matching M7 calibration); answer quality / EvidenceGate behavior; p50/p95 latency; memory footprint; model load time (cold vs warm). Latency is judged relatively against the production baseline — absolute numbers on the ~3K-chunk corpus would be misleading (report figures come from MS MARCO-scale benchmarks).
- **Kill gate — acceptance thresholds (frozen before the benchmark, baseline-relative).**
  - **MPI (minimum practical improvement):** nDCG@5 delta >= +1 pp over baseline. Named in the ADR so "why +1?" is never reopened: it is a product/engineering minimum meaningful improvement threshold, not a statistical construct.
  - **MAR (maximum acceptable regression):** MRR delta >= -0.5 pp (first correct result must not degrade beyond measurement noise); verified-rate delta >= -1 pp (answer quality must not regress).
  - **Abstention is a hard invariant:** `no_evidence` behavior on abstention queries must be identical with reranking off and on — no tolerance.
  - Bootstrap CI (paired, query-level) is reported but is never a decision criterion — with a small gold set it would make the decision itself unstable.
- **Determinism requirement.** Re-running over the same candidate artifact yields the same ordering (ADR-0044 contract) and the same benchmark fingerprint reproduces results; the benchmark report includes the re-run as evidence.
- **Selection rule (deterministic).** Among accepted combinations, the highest quality wins; if quality deltas are within the 5% tolerance, the smallest N wins (lowest operational cost). If no combination passes the thresholds, M8 closes here (ADR-0043 closure rule): production path unchanged, closure recorded in an ADR.
- **Operational budgets.** p95 latency and memory budgets must not be exceeded; exact budget values are frozen in the benchmark manifest before the probe starts, alongside the fingerprint and gold set version, and are part of the acceptance criteria.

## Rationale

- The probe answers "does the production path benefit from reranking?" — only measurable with the single-variable design above; any retrieval-side change would corrupt the result.
- Frozen, baseline-relative thresholds keep the gate deterministic and pre-announced: absolute targets would become free or impossible if the baseline shifts.
- Candidate artifacts make comparisons fully isolated and re-runnable, which is the strongest defense of benchmark validity.

## Consequences

- The eval harness gains: artifact recording (candidate lists per query), per-combination metric collection, and the MPI/MAR gate evaluation. Probe scripts/commands are benchmark tooling, not production code (ADR-0042 distinction).
- Acceptance produces (model, N) frozen values; rejection produces a closure ADR. Either way ADR-0046 defines what happens next.
