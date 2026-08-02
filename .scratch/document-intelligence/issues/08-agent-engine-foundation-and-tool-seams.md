# 08 — Agent Engine Foundation & Tool Seams

**What to build:**
The `internal/agent` Bounded Context featuring `AgentEngine`, `Planner`, `Executor`, `AgentPolicy` (`MaxSteps`, `TokenBudget`, `ApprovalSeam`), short-term/long-term `Memory` seams, and `Tool` interface abstractions (`KnowledgeTool`, `GraphTool`, `VerificationTool`).

**Blocked by:** 01 — QA Core, 07 — Graph Bounded Context.

**Status:** ready-for-agent

- [ ] Define `Tool` interface seam (`Name`, `Description`, `Execute(ctx, input)`).
- [ ] Implement `KnowledgeTool`, `GraphTool`, and `VerificationTool` adapters.
- [ ] Implement `Planner` and `Executor` pipeline adhering to `AgentPolicy` limits.
- [ ] Define `Memory` interface seam and `InMemoryAgentMemory` adapter.
- [ ] Add `RequestRouter` routing simple queries to `AnswerEngine` and complex multi-step research requests to `AgentEngine`.
- [ ] Add unit tests verifying multi-step plan execution, tool invocation, policy limit enforcement, and request routing.
