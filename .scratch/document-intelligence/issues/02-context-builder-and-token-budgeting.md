# 02 — ContextBuilder & Token Budgeting Engine

**What to build:**
The `ContextBuilder` component in `internal/qa/context` assembling prompt-ready `ContextWindow` structures (`Sources`, `Content`, `TokenCount`), implementing token budgeting, deduplication, section re-ordering, and immutable citation marker (`[Ref N]`) injection.

**Blocked by:** 01 — QA Orchestration Core.

**Status:** ready-for-agent

- [ ] Define `ContextWindow` and `SourceReference` domain models.
- [ ] Define `TokenCounter` interface seam (`Count(text string) int`).
- [ ] Implement `DefaultContextBuilder` with configurable token budget limit and `[Ref N]` marker assignment.
- [ ] Add unit tests verifying token budget truncations, duplicate chunk stripping, and citation marker stability.
