# 10 — Production HTTP REST/SSE API Surface & CLI Tool

**What to build:**
The `cmd/arc-server` entrypoint (Fiber HTTP REST & SSE Streaming API) and `cmd/arc` entrypoint (CLI tool for developer workflows, terminal QA, and CI/CD pipelines).

**Blocked by:** 05 — SSE Streaming & Async QA Job, 09 — MCP Server.

**Status:** ready-for-agent

- [ ] Implement `cmd/arc-server/main.go` Fiber HTTP server.
- [ ] Implement REST endpoints (`/api/v1/spaces`, `/api/v1/documents/upload`, `/api/v1/indexing/jobs`, `/api/v1/research/jobs`).
- [ ] Implement SSE Streaming endpoint (`/api/v1/qa/stream`) wrapping `StreamingAnswerEngine`.
- [ ] Implement `cmd/arc/main.go` CLI tool (`arc inspect`, `arc ask`, `arc research`).
- [ ] Add end-to-end HTTP integration tests and CLI command tests.
