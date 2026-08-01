# 1. Use Firecrawl Open-Source PDF Pipeline for Parsing & Markdown Conversion

* Status: Accepted
* Date: 2026-08-01

## Context and Problem Statement

PDF Inspector V1 requires reliable document parsing, page-aware text extraction, OCR detection, and Markdown conversion as the entry point of ARCA's knowledge ingestion pipeline. Building a robust PDF parser and Markdown converter from scratch requires significant engineering effort and ongoing maintenance.

## Decision Drivers

* Speed to market for V1 knowledge ingestion.
* Reliability of document layout reconstruction and Markdown formatting.
* Ability to focus ARCA engineering effort on semantic structure analysis, chunking, and graph readiness rather than low-level PDF byte parsing.

## Considered Options

* Option A: Build custom PDF parsing engine from scratch using low-level PDF libraries (PDF.js, PyMuPDF, etc.).
* Option B: Leverage Firecrawl's open-source PDF extraction pipeline as the foundation, extending it with ARCA-specific semantic processing.

## Decision Outcome

Chosen Option: Option B.

### Positive Consequences

* Immediate access to a proven, open-source pipeline for PDF parsing and Markdown conversion.
* Clean separation of concerns: Firecrawl handles raw extraction & Markdown conversion; ARCA PDF Inspector handles semantic structure analysis, sectioning, and chunking.

### Negative Consequences

* Dependency on Firecrawl's extraction pipeline behavior and update cycles.
