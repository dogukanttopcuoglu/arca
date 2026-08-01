# 01 — Firecrawl HTTP Service Integration Client & Test Seam

**What to build:** An HTTP client library in Go that streams PDF files to the isolated Firecrawl microservice and receives raw Markdown and document layout extraction payloads, including configurable request timeouts, exponential backoff retries, and error mapping to `SERVICE_UNAVAILABLE`. Verifiable via an `httptest.Server` test seam.

**Blocked by:** 00 — Project Skeleton & Development Environment

**Status:** ready-for-agent

- [ ] Go HTTP client successfully POSTs PDF streams to the Firecrawl endpoint.
- [ ] Response payloads containing raw Markdown, layout JSON, and parser metadata are parsed into Go structures.
- [ ] Request timeouts and exponential backoff retry policies operate predictably during network glitches.
- [ ] Integration tests using `httptest.Server` validate client behavior against mock Firecrawl responses.
