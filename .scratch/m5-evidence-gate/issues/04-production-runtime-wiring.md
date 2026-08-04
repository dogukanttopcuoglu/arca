---
title: 04 — Production EvidenceGate wiring
feature: m5-evidence-gate
status: ready-for-agent
created: 2026-08-04
blocked-by: 03 — LLM EvidenceGate adapter
---

# 04 — Production EvidenceGate wiring

## Goal

Ensure every production synchronous `AnswerEngine` composition root supplies a real EvidenceGate.

## Scope

- Wire the real adapter in CLI, server, and MCP production composition roots.
- Preserve consumer code and streaming code unchanged.
- Allow nil only in tests or explicitly legacy/offline composition.
- Align production `RETRIEVAL_MIN_SCORE` default with frozen M4 value `0.6`.
- Do not add feature flags or runtime policy routing.

## Files/modules affected

- `cmd/arc/cli/runtime.go`
- `cmd/arc-server/main.go`
- `cmd/arc-mcp/server/server.go`
- Configuration tests

## Dependencies

- 03 — LLM EvidenceGate adapter

## Acceptance criteria

- Production composition cannot silently omit the gate.
- CLI, server, and MCP engines receive the real adapter.
- Agent behavior changes only through the shared AnswerEngine.
- Streaming remains untouched.
- The `0.6` default is covered by configuration tests.
