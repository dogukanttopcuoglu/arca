# 04 — Citation Extraction & Structural Verification Engine

**What to build:**
The `CitationExtractor` and `VerificationPipeline` in `internal/qa/citation` and `internal/qa/verification` parsing inline reference markers (`[Ref N]`), mapping them to immutable `ContextWindow` sources, computing `VerificationReport` (`TotalClaims`, `VerifiedClaims`, `InvalidReferences`), and providing the Phase 2 `EntailmentChecker` seam.

**Blocked by:** 02 — ContextBuilder, 03 — Prompt Layer & LLM Seam.

**Status:** ready-for-agent

- [ ] Define `Answer`, `AnswerCitation`, and `VerificationReport` domain models.
- [ ] Implement `CitationExtractor` using regex/AST marker detection and `ContextWindow` mapping.
- [ ] Implement `VerificationPipeline` and `StructuralVerifier` Phase 1 engine.
- [ ] Define Phase 2 `EntailmentChecker` interface seam and `MockEntailmentChecker`.
- [ ] Add unit tests verifying marker extraction, invalid reference detection (`[Ref 99]`), and report calculation.
