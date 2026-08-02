# 05 — SSE Answer Streaming & Async QA Job Architecture

**What to build:**
Dual-Mode execution engine featuring `StreamingAnswerEngine` (SSE token streaming with stream finalization verification) and `AsyncAnswerEngine` (`QAJob` state machine and background worker) in `internal/qa/job`.

**Blocked by:** 01 — QA Core, 04 — Citation Verification.

**Status:** ready-for-agent

- [ ] Define `AnswerStreamChunk` struct and `StreamChunkType` enum (`token`, `verification`, `error`).
- [ ] Define `QAJob` model and status state machine (`Pending` -> `Planning` -> `Retrieving` -> `Generating` -> `Verifying` -> `Completed`).
- [ ] Implement `StreamingAnswerEngine` adapter wrapping `AnswerEngine` core logic.
- [ ] Implement `AsyncAnswerEngine` and `QAJobWorker` background worker.
- [ ] Add unit tests verifying SSE channel streaming and background QA job state transitions.
