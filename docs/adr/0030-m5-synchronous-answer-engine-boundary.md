# M5 synchronous AnswerEngine boundary

Status: accepted

M5 orchestration applies only to the synchronous `AnswerEngine` path. Streaming, MCP, and agent consumers receive the behavior through their shared synchronous engine where applicable, but require no consumer-specific orchestration changes; streaming parity and consumer-specific benchmarks remain follow-up work.

## Consequences

- `AnswerEngine` is the canonical M5 execution path.
- Every production `AnswerEngine` composition root must provide a real `EvidenceGate`.
- Tests and explicitly legacy/offline composition may use a nil gate.
