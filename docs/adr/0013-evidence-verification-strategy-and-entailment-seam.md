# 0013: Evidence Verification Strategy & Entailment Seam

- **Status:** Accepted
- **Date:** 2026-08-02
- **Deciders:** Staff Software Architect & Lead Engineer

## Context and Problem Statement

Validating citation existence (Structural Verification) guarantees that a chunk exists on a specific page, but does not guarantee that the generated text logically follows from the chunk (Entailment). We must design an evidence verification pipeline without introducing prohibitive latency in MVP.

## Decision Drivers

- **Phased Rollout**: MVP requires zero-latency structural verification, with architectural seams prepared for semantic NLI entailment checking.
- **Pluggable Architecture**: `VerificationPipeline` orchestrates verification modules without coupling `AnswerEngine` to specific NLI models.

## Decided Options

### Option A: Phased Evidence Verification (ACCEPTED)
- **Phase 1 (Structural Verification MVP)**: `CitationExtractor` validates reference existence, chunk IDs, page numbers, and builds `VerificationReport`. Zero additional LLM latency.
- **Phase 2 (Entailment Verification Seam)**: `internal/qa/verification` defines `EntailmentChecker` (`CheckEntailment(ctx, claim, sourceText) (EntailmentScore, error)`). Can be plugged into `VerificationPipeline` for high-confidence enterprise modes.

### Option B: Mandatory Synchronous Entailment Verification on MVP (REJECTED)
Executing NLI entailment checks on every generated sentence before returning initial MVP answers, adding 5-10s latency per request.

## Consequences

### Positive
- MVP remains fast (~2-4s latency).
- Future-proof seam for NLI semantic entailment checks.
