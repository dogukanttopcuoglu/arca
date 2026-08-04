# M5 EvidenceGate is separate from EntailmentChecker

Status: accepted

`EvidenceGate` is a pre-generation, query-level seam that decides whether the final context supports the original query. It remains separate from `EntailmentChecker`, which performs post-generation, claim-level semantic verification; sharing the contract would overload verification with a different responsibility and encourage per-chunk work.

## Consequences

- `EvidenceGate` belongs to the `qa` orchestration context.
- The gate returns explicit `supported`, `unsupported`, or `gate_error` outcomes.
- Both components may later share provider adapters, but neither owns the other's decision.
