# M8 reranking production activation: benchmarked wiring into the graph slot, fingerprint-locked configuration

Status: accepted

Reranking is activated in production only after ADR-0045 acceptance, wired into the graph retrieval slot, and locked to the exact benchmark-accepted configuration. Until acceptance, nothing changes; rejection keeps production byte-identical to today.

## Decisions

- **Wiring after acceptance only.** `AnswerEngine`'s retriever wiring gains the wrapper by composition, not by a new engine concept: `WithGraphRetriever(NewRerankedRetriever(graphFusionRetriever, rerankConfig))`. The M7 slot structure and orchestration gate are unchanged; rerank sits above the graph fusion path the gate selects. The dense path is untouched because the benchmark measured only GraphFusion (ADR-0045 delta isolation) — what is measured in the benchmark is what is activated in production, so benchmark and live behavior cannot diverge.
- **Wrapper vs wiring (normative).** `RerankedRetriever` is a generic domain execution component (ADR-0044) that can wrap any retriever. Its wiring over `GraphFusionRetriever` is a production decision taken from the M8 benchmark result — the benchmark does not make the wrapper graph-specific, it activates it on the graph path. Benchmarking another path later is a composition change only, no new abstraction.
- **Configuration.** `RETRIEVAL_RERANK_MODEL` — empty string means the feature is off (no separate `ENABLED` flag; the empty value is the off state, mirroring the M7 weight-at-zero pattern). `RETRIEVAL_RERANK_CANDIDATE_N` — controls only the wrapper's internal candidate budget; AnswerEngine and callers keep TopK semantics unchanged.
- **Rollback.** One configuration change (empty the model value) returns to M7 behavior — rollback never requires a code change.
- **Fingerprint <-> config traceability (normative, release criterion).** The benchmark artifact fingerprint (ADR-0045) must match the production configuration: if the benchmark accepted Model=X, N=50, production may only open with Model=X, N=50. Changing the model, CandidateN, or any measured parameter invalidates the acceptance and requires a new benchmark; an existing acceptance never carries over automatically. The benchmark result and the production configuration are one-to-one traceable.
- **Rejection behavior.** On rejection, none of the above is introduced: no config keys, no wiring; the eval surface (probe scripts/commands) remains as an experiment surface only — measurement tooling never shares a decision mechanism with production config (ADR-0042 distinction).

## Rationale

- "Measured things go to production" (M4-M7 discipline) holds end-to-end: benchmark -> frozen config -> activation; anything else would introduce unmeasured behavior into the live path.
- The empty-model off state and single-config rollback keep the feature trivially reversible, which is the safety requirement for any post-retrieval addition.
- Fingerprint locking prevents silent drift between the accepted artifact and the live configuration.

## Consequences

- After acceptance: composition root builds the wrapper with the frozen (model, N) values; a deployment decision (in-process inference vs sidecar service) is deferred and recorded as a follow-up decision outside this ADR's scope.
- This closes the M8 decision set (ADR-0043...0046). Next: `/to-spec` collapses decisions into a buildable plan, `/to-tickets` splits the probe and activation work, `/implement` builds it TDD-first with the ADR-0045 thresholds as the acceptance gate.
