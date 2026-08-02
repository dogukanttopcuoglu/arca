# 0017: Agentic Knowledge Orchestration Layer Architecture

- **Status:** Accepted
- **Date:** 2026-08-02
- **Deciders:** Staff Software Architect & Lead Engineer

## Context and Problem Statement

To evolve ARC into a Document Intelligence Operating System, it must execute multi-step research plans, use internal/external tools (Knowledge Search, Graph Traversal, GitHub, MCP), and handle autonomous background goals safely.

## Decision Drivers

- **Reasoning vs. Domain Data**: Agent controls action sequencing and reasoning, while domain modules (`internal/qa`, `internal/graph`) manage data and logic.
- **Request Router**: Simple queries execute via `AnswerEngine` (zero agent overhead); complex multi-step queries activate `AgentEngine`.
- **Safety & Approval**: Agent execution requires strict policies (`MaxSteps`, `TokenBudget`, `ApprovalSeam`).

## Decided Options

### Option A: `internal/agent` Autonomous Controller (ACCEPTED)
- `internal/agent` is an independent Bounded Context (`engine.go`, `planner.go`, `executor.go`, `policy.go`, `memory/`, `tool/`).
- Every system capability is encapsulated as a `Tool` interface (`KnowledgeTool`, `GraphTool`, `VerificationTool`, `MCPTool`, `GitHubTool`).
- Uses a `Planner` + `Executor` pipeline for predictable research step execution.
- Security Policy: Enforces `MaxSteps`, `MaxToolCalls`, `TokenBudget`, and `RequireApproval` for destructive actions (e.g. GitHub issue creation).

## Consequences

### Positive
- Enables autonomous multi-step research over documents.
- Simple queries incur zero agent latency overhead.
- Safe tool execution with human-in-the-loop approval seams.
