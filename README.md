# ARCA - PDF Inspector

PDF Inspector is the entry-point stage of the ARC knowledge ingestion pipeline. It transforms PDF documents into structured, canonical knowledge artifacts (`PDFInspectionResult`) ready for downstream vector embedding generation, Knowledge Graph construction, and RAG retrieval.

## Architecture

- **Core Backend**: Go (Golang) + Fiber Framework
- **PDF Extraction Engine**: Firecrawl Open-Source PDF Extraction Pipeline (Node.js / TypeScript microservice)
- **Communication Boundary**: HTTP REST API / Docker Container
- **Intermediate Representation**: `PDFInspectionResult`

## Project Structure

```text
internal/pdfinspector/
├── model/           # Canonical domain structures & JSON schemas
├── config/          # Environment-driven configuration
├── logger/          # Structured JSON logging
├── firecrawl/       # Firecrawl HTTP client & test seam
├── semantic/        # Semantic tree reconstruction engine
├── chunking/        # Hierarchical semantic chunking engine
├── assets/          # Document asset (tables, figures, code, equations) & citation extractor
├── diagnostics/     # Resiliency & diagnostics aggregator
└── inspector/       # Core PDFInspector orchestrator & DI
```

## Running Tests

Run all unit and integration tests:

```bash
go test ./...
```

## Local Development (Docker Compose)

Start the local Firecrawl PDF extraction service:

```bash
docker-compose up -d
```
