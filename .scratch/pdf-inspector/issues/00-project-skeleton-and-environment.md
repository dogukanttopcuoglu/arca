# 00 — Project Skeleton & Development Environment

**What to build:** Foundational Go package layout (`internal/pdfinspector/...`), core interfaces (`FirecrawlClient`, `SemanticProcessor`, `ChunkingEngine`, `AssetExtractor`), dependency injection, configuration, structured logging, reusable test fixtures (sample, encrypted, corrupted, table/equation PDFs), Docker Compose for Firecrawl local dev, and integration bootstrap verification.

**Blocked by:** None — can start immediately.

**Status:** completed

- [x] Package layout under `internal/pdfinspector/` is established.
- [x] Core domain interfaces and dependency injection wire-up are defined.
- [x] Structured logging and configuration system (Firecrawl URL, timeouts, max page count) are operational.
- [x] Reusable test fixtures for various PDF types (valid, encrypted, corrupted) are committed.
- [x] Docker Compose file for local Firecrawl service setup is available and reachable via Go test bootstrap.
