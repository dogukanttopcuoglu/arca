package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"arca/internal/pdfinspector/assets"
	"arca/internal/pdfinspector/chunking"
	"arca/internal/pdfinspector/config"
	"arca/internal/pdfinspector/diagnostics"
	"arca/internal/pdfinspector/firecrawl"
	"arca/internal/pdfinspector/inspector"
	"arca/internal/pdfinspector/model"
	"arca/internal/pdfinspector/semantic"

	indexingmodel "arca/internal/indexing/model"
	"arca/internal/indexing/provider"
	"arca/internal/indexing/store"
	"arca/internal/indexing/worker"
)

const banner = `
===================================================================
               ARC PDF INSPECTOR V1 - PIPELINE DEMO                
===================================================================
`

func main() {
	pdfPath := flag.String("pdf", "", "Path to input PDF file (optional, uses sample document if empty)")
	outputPath := flag.String("out", "inspection_result.json", "Output JSON path for PDFInspectionResult")
	docIDFlag := flag.String("doc-id", "", "Stable document ID (default: derived from input filename)")
	mockService := flag.Bool("mock", false, "Force simulated mock Firecrawl service seam (set true if Docker service is offline)")
	index := flag.Bool("index", false, "Index inspected chunks into an in-memory vector store")
	query := flag.String("query", "", "Run a retrieval query against the indexed chunks")
	firecrawlURL := flag.String("url", "", "Firecrawl service URL (default: FIRECRAWL_BASE_URL env or http://localhost:3002)")
	flag.Parse()

	fmt.Print(banner)

	cfg := config.LoadFromEnv()
	if *firecrawlURL != "" {
		cfg.FirecrawlBaseURL = *firecrawlURL
	}
	if cfg.FirecrawlBaseURL == "" {
		cfg.FirecrawlBaseURL = "http://localhost:3002"
	}

	var server *httptest.Server
	if *mockService {
		fmt.Println("[+] Running in MOCK mode (simulated Firecrawl PDF extraction service)...")
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(sampleExtractionJSON()))
		}))
		defer server.Close()
		cfg.FirecrawlBaseURL = server.URL
	} else {
		fmt.Printf("[+] REAL FIRECRAWL MODE: Connecting to Firecrawl HTTP service at: %s\n", cfg.FirecrawlBaseURL)
	}

	// 1. Instantiate pipeline components via Dependency Injection
	client := firecrawl.NewHTTPClient(cfg.FirecrawlBaseURL)
	processor := semantic.NewProcessor()
	chunker := chunking.NewEngine()
	extractor := assets.NewExtractor()
	aggregator := diagnostics.NewAggregator()

	pdfInspector := inspector.NewPDFInspector(cfg, client, processor, chunker, extractor, aggregator)

	// 2. Read input PDF stream
	var inputReader *strings.Reader
	fileName := "sample_document.pdf"

	if *pdfPath != "" {
		data, err := os.ReadFile(*pdfPath)
		if err != nil {
			fmt.Printf("[-] Error reading input PDF file %s: %v\n", *pdfPath, err)
			os.Exit(1)
		}
		inputReader = strings.NewReader(string(data))
		fileName = filepath.Base(*pdfPath)
		fmt.Printf("[+] Processing user provided PDF: %s (%d bytes)\n", fileName, len(data))
	} else {
		// Valid sample PDF header payload
		sampleContent := "%PDF-1.4 Header Sample PDF\n" + sampleExtractionJSON()
		inputReader = strings.NewReader(sampleContent)
		fmt.Printf("[+] Processing sample demonstration document: %s\n", fileName)
	}

	docID := *docIDFlag
	if docID == "" {
		docID = filepath.Base(strings.TrimSuffix(fileName, filepath.Ext(fileName)))
	}
	fmt.Printf("[+] Document ID: %s\n", docID)

	// 3. Execute top-level pipeline
	fmt.Println("[+] Executing InspectPDF orchestrator pipeline...")
	startTime := time.Now()

	result, err := pdfInspector.InspectPDF(context.Background(), docID, inputReader)
	duration := time.Since(startTime)

	if err != nil {
		fmt.Printf("\n[-] Pipeline execution failed: %v\n", err)
		if result != nil && result.Diagnostics.Status == model.StatusFailed {
			fmt.Printf("    Diagnostics Errors: %v\n", result.Diagnostics.Errors)
		}
		os.Exit(1)
	}

	// 4. Print Summary Results
	fmt.Println("\n===================================================================")
	fmt.Println("                      INSPECTION RESULT SUMMARY                    ")
	fmt.Println("===================================================================")
	fmt.Printf(" Schema Version   : %s\n", result.SchemaVersion)
	fmt.Printf(" Document Title   : %s\n", result.Document.Title)
	fmt.Printf(" Author           : %s\n", result.Document.Author)
	fmt.Printf(" Page Count       : %d\n", result.Document.PageCount)
	fmt.Printf(" Pipeline Status  : %s\n", strings.ToUpper(result.Diagnostics.Status))
	fmt.Printf(" Engine / Version : %s (v%s)\n", result.Diagnostics.ExtractionEngine, result.Diagnostics.ExtractionVer)
	fmt.Printf(" Processing Time  : %d ms (wall clock: %v)\n", result.Diagnostics.ProcessingTimeMs, duration)
	fmt.Println("-------------------------------------------------------------------")
	fmt.Printf(" Root Nodes       : %d section(s) in Semantic Tree\n", len(result.SemanticTree.RootNodes))
	fmt.Printf(" Knowledge Chunks : %d chunk(s) generated\n", len(result.Chunks))
	fmt.Println("-------------------------------------------------------------------")
	fmt.Println(" Extracted Document Assets:")
	fmt.Printf("   - Tables       : %d\n", len(result.Assets.Tables))
	fmt.Printf("   - Figures      : %d\n", len(result.Assets.Figures))
	fmt.Printf("   - Code Blocks  : %d\n", len(result.Assets.CodeBlocks))
	fmt.Printf("   - Equations    : %d\n", len(result.Assets.Equations))
	fmt.Printf("   - Citations    : %d\n", len(result.Assets.Citations))
	fmt.Println("-------------------------------------------------------------------")
	fmt.Printf(" Diagnostics Warnings : %d\n", len(result.Diagnostics.Warnings))
	for idx, w := range result.Diagnostics.Warnings {
		fmt.Printf("   [%d] %s\n", idx+1, w)
	}
	fmt.Println("===================================================================")

	// 5. Serialize and write JSON output
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Printf("[-] Failed to serialize result to JSON: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(*outputPath, jsonBytes, 0644); err != nil {
		fmt.Printf("[-] Failed to write JSON output to %s: %v\n", *outputPath, err)
		os.Exit(1)
	}

	fmt.Printf("\n[✓] Inspection result successfully exported to: %s (%d bytes)\n\n", *outputPath, len(jsonBytes))

	// 6. End-to-end validation: inspect → index → retrieve
	if *index || *query != "" {
		fmt.Println("===================================================================")
		fmt.Println("                 KNOWLEDGE INDEXING (IN-MEMORY)                    ")
		fmt.Println("===================================================================")

		indexingProvider := provider.NewMockEmbeddingProvider("mock-provider", "mock-model-v1", 1536)
		vectorStore := store.NewInMemoryVectorStore()
		indexingWorker := worker.NewIndexingWorker(indexingProvider, vectorStore)

		indexCtx := context.Background()
		jobObj, err := indexingWorker.ExecuteSync(indexCtx, result.Document.DocumentID, result.Chunks)
		if err != nil {
			fmt.Printf("[-] Indexing failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[+] Document ID   : %s\n", jobObj.DocumentID)
		fmt.Printf("[+] Indexed chunks: %d\n", jobObj.IndexedChunks)
		fmt.Printf("[+] Skipped chunks: %d\n", jobObj.SkippedChunks)
		fmt.Printf("[+] Deleted points: %d\n", jobObj.DeletedChunks)
		fmt.Printf("[+] Provider      : %s (%s)\n", jobObj.EmbeddingProvider, jobObj.EmbeddingModel)

		if *query != "" {
			fmt.Println("-------------------------------------------------------------------")
			fmt.Printf("[+] Retrieval query: %q\n", *query)

			queryEmb, err := indexingProvider.GenerateEmbeddings(indexCtx, []string{*query})
			if err != nil {
				fmt.Printf("[-] Query embedding failed: %v\n", err)
				os.Exit(1)
			}

			matches, err := vectorStore.SearchVector(indexCtx, store.VectorSearchQuery{
				Vector: queryEmb.Vectors[0],
				TopK:   5,
				Filter: indexingmodel.MetadataFilter{DocumentIDs: []string{result.Document.DocumentID}},
			})
			if err != nil {
				fmt.Printf("[-] Retrieval failed: %v\n", err)
				os.Exit(1)
			}

			if len(matches) == 0 {
				fmt.Println("[-] No matching chunks found.")
			}
			for i, m := range matches {
				fmt.Printf("   [%d] score=%.4f pages=%v section=%q chunk=%s\n",
					i+1, m.Score, m.Metadata.PageNumbers, m.Metadata.SectionPath, m.Metadata.ChunkID)
			}
		}
		fmt.Println("===================================================================")
	}
}

func sampleExtractionJSON() string {
	return "{\n" +
		`  "markdown": "# ARC System Technical Manual\n\nWelcome to the ARC Knowledge Ingestion pipeline documentation.\n\n## Architecture Overview\n\nThe ARC PDF Inspector ingests raw PDF files and outputs canonical inspection results.\n\n### Key Subsystems\n\nHere is a list of supported asset types:\n\n| Asset Type | Description | Schema |\n| --- | --- | --- |\n| Tables | Markdown grids | Table |\n| Code Blocks | Snippets | CodeBlock |\n| Equations | LaTeX formulas | Equation |\n\n### Code Implementation Example\n\nBelow is a sample Go snippet for constructing the inspector:\n\n` + "```go\\nfunc NewInspector() *PDFInspector {\\n    return &PDFInspector{}\\n}\\n```" + `\n\n### Mathematical Formulation\n\nThe token bound algorithm evaluates total character weights as:\n\n$$E = mc^2 + \\sum_{i=1}^{n} w_i$$\n\nAccording to Smith et al. [1], semantic boundaries prevent fragmentation.\n\n## References\n\n[1] Smith, J. et al. (2025). Knowledge Ingestion & Semantic Preservation.",` + "\n" +
		`  "json_layout": {` + "\n" +
		`    "pages": [` + "\n" +
		`      {"page_number": 1, "markdown": "# ARC System Technical Manual\n\nWelcome to the ARC Knowledge Ingestion pipeline documentation.\n\n## Architecture Overview"},` + "\n" +
		`      {"page_number": 2, "markdown": "### Key Subsystems\n\n| Asset Type | Description | Schema |\n| --- | --- | --- |\n| Tables | Markdown grids | Table |\n| Code Blocks | Snippets | CodeBlock |\n| Equations | LaTeX formulas | Equation |\n\n` + "```go\\nfunc NewInspector() *PDFInspector {\\n    return &PDFInspector{}\\n}\\n```" + `\n\n$$E = mc^2 + \\sum_{i=1}^{n} w_i$$\n\nAccording to Smith et al. [1]."}` + "\n" +
		`    ]` + "\n" +
		`  },` + "\n" +
		`  "metadata": {` + "\n" +
		`    "title": "ARC System Technical Manual",` + "\n" +
		`    "author": "ARC Engineering Team",` + "\n" +
		`    "page_count": 2,` + "\n" +
		`    "fonts": ["Inter", "JetBrains Mono"],` + "\n" +
		`    "searchable": true` + "\n" +
		`  },` + "\n" +
		`  "ocr_applied": false` + "\n" +
		"}"
}
