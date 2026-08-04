# M5 performs semantic abstention before generation

Status: accepted

After retrieval, decomposition/merge, and final `ContextWindow` construction, M5 evaluates the original user query against that exact context before invoking the answer LLM. Empty retrieval preserves the existing immediate `no_evidence` path; retrieved but unsupported context returns `StatusNoEvidence` and skips generation.

Operational gate failures are not semantic abstentions: the gate retries once, then returns a typed error and no `Answer` without invoking generation. Malformed or ambiguous provider output is a `gate_error`.

## Consequences

- The gate can prevent unnecessary answer-generation calls.
- The first benchmark uses Gold Set v1.1 `expected_no_evidence` labels, while documenting that they are query-level rather than context-window-level ground truth.
- Deterministic fake-gate tests and real-provider evaluation remain separate.
