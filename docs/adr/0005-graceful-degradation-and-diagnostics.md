# 5. Graceful Degradation and Diagnostics Resiliency Policy

* Status: Accepted
* Date: 2026-08-01

## Context and Problem Statement

PDF parsing and asset extraction across complex multi-page PDF documents is subject to partial failures (OCR errors, malformed tables, isolated corrupted pages, HTTP timeouts). A fail-fast policy for non-fatal errors discards valuable extracted knowledge unnecessarily.

## Decision Drivers

* Knowledge preservation over perfect extraction.
* Observability for downstream services regarding data completeness.
* Preventing pipeline failures due to localized document anomalies.

## Decision Outcome

Chosen Option: Graceful Degradation with Detailed Diagnostics.

* Encrypted and unreadable corrupted PDFs fail fast with `ENCRYPTED_DOCUMENT` or `INVALID_DOCUMENT`.
* Page/asset extraction anomalies are caught, recorded under `diagnostics.warnings` / `skippedPages`, and the pipeline produces a `partial_success` status payload.
* Firecrawl HTTP integration utilizes timeouts, exponential backoff retries, and circuit breakers.
* Large documents are processed incrementally via page batching and streaming.

### Positive Consequences

* Maximizes successfully extracted knowledge.
* Downstream consumer services gain explicit diagnostic flags to decide how to handle partial pages.
