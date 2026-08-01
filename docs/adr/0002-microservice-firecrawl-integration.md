# 2. Integrate Firecrawl as a Dedicated Microservice over HTTP

* Status: Accepted
* Date: 2026-08-01

## Context and Problem Statement

ARC's core backend is implemented in Go, whereas Firecrawl's PDF extraction pipeline is implemented in Node.js/TypeScript. We need a clean, maintainable integration strategy between the two runtimes.

## Decision Drivers

* Polyglot isolation (Go backend vs Node.js runtime).
* Independent upgradability and replaceability of the PDF extraction engine.
* Horizontal scalability for resource-intensive PDF processing.
* Clean separation of concerns between document parsing (Firecrawl) and semantic knowledge processing (ARC).

## Considered Options

* Option A: Port Firecrawl PDF extraction logic directly to Go.
* Option B: Run Firecrawl as a subprocess/CLI executable invoked by Go.
* Option C: Run Firecrawl as a dedicated local microservice / Docker container exposing a clean HTTP API.

## Decision Outcome

Chosen Option: Option C (Dedicated Microservice / Docker Container).

### Positive Consequences

* Completely decouples ARC's Go backend from Node.js dependencies.
* Firecrawl engine can be updated or swapped without touching Go backend code.
* Dedicated HTTP API provides a strict seam for testing and mocking.

### Negative Consequences

* Requires service orchestration (e.g. Docker Compose or local network management) during development and deployment.
