# 01 — Reranker seam and ordering contract

**What to build:** A new `Reranker` seam (ADR-0044) that abstracts only model behavior — `Rerank(ctx, query, candidates)`-shaped — with the ordering contract enforced: absolute scores carry no meaning, adapters need not share a score scale, and the only guarantee is deterministic ordering with ChunkID ASC tie-break. The seam carries contract tests that prove ordering determinism, tie-break behavior, and scale independence (two fake adapters producing different score scales yield the same ordering for the same preferences).

**Blocked by:** None — can start immediately.

**Status:** resolved

- [ ] `Reranker` seam surface exists per ADR-0044 (ordering-only contract, no retrieval concerns)
- [ ] Contract tests: same query + same candidates -> same ordering
- [ ] Contract tests: ChunkID ASC tie-break determinism
- [ ] Contract tests: score-scale independence between adapters (no shared scale assumed)
- [ ] No filtering/thresholding capability leaks out of the seam (ordering contract forbids it)

## Comments
