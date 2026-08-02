# 0014: RAG Execution Model — Streaming & Async Job Architecture

- **Status:** Accepted
- **Date:** 2026-08-02
- **Deciders:** Staff Software Architect & Lead Engineer

## Context and Problem Statement

ARC serves both interactive real-time chat queries ("What does Chapter 1 say?") and heavy multi-document research tasks ("Synthesize all 50 chapters"). A single execution model causes either poor chat UX or HTTP timeouts on long-form reports.

## Decision Drivers

- **User Experience**: Interactive chat requires low-latency token streaming (SSE).
- **Scalability & Resiliency**: Long-running synthesis requires background job processing, state machines, and polling/webhooks.
- **Pattern Reuse**: Deep research jobs should reuse the `IndexingJob` state machine pattern (`internal/indexing/job`).

## Decided Options

### Option A: Dual-Mode Execution Architecture (ACCEPTED)
- **Interactive Mode (SSE Streaming)**: `StreamingAnswerEngine` streams tokens via Server-Sent Events (`<-chan AnswerStreamChunk`). Citation verification runs during stream finalization.
- **Deep Research Mode (Async QA Job)**: `AsyncAnswerEngine` manages `QAJob` state machine (`Pending` -> `Planning` -> `Retrieving` -> `Generating` -> `Verifying` -> `Completed`) in `internal/qa/job`.
- **Shared Core**: Both adapters wrap the identical `AnswerEngine` core logic.

## Consequences

### Positive
- Chat UI achieves immediate token rendering (<300ms time-to-first-token).
- Heavy multi-document synthesis avoids HTTP connection timeouts.
