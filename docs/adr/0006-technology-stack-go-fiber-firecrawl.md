# 6. Comprehensive Technology Stack: Fiber, fasthttp, Zap, Viper, Validator

* Status: Accepted
* Date: 2026-08-01

## Context and Problem Statement

ARC PDF Inspector requires a production-ready, scalable, observable, and high-performance engineering foundation that minimizes future rewrites while standardizing across the ARC backend ecosystem.

## Decision Drivers

* High HTTP throughput and low overhead (Go + Fiber + fasthttp).
* Enterprise-grade, high-performance structured logging (Uber Zap).
* Flexible environment and file configuration (Viper).
* Standardized validation and assertions (`go-playground/validator`, `testify`).
* Clean microservice boundary for PDF extraction (Node.js/TypeScript Firecrawl in Docker).

## Decision Outcome

Chosen Package Standards:

* Core Backend: Go (Golang)
* HTTP Framework: **Fiber**
* HTTP Client: **fasthttp**
* JSON Serialization: Standard `encoding/json`
* Configuration: **Viper** (`github.com/spf13/viper`)
* Logging: **Uber Zap** (`go.uber.org/zap`)
* Validation: **go-playground/validator/v10**
* UUID: **google/uuid**
* Testing & Assertions: Standard `testing` package with **testify** and `net/http/httptest`
* Extraction Service: Firecrawl (Node.js / TypeScript) via Docker Compose

### Positive Consequences

* Performance-optimized across HTTP ingress and egress layers (`fasthttp`).
* Observable by default with context-aware Zap logging.
* Highly testable architecture with testify assertions and HTTP test seams.
