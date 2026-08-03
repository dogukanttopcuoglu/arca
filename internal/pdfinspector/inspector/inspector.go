package inspector

import (
	"bytes"
	"context"
	"io"
	"time"

	"arca/internal/pdfinspector/assets"
	"arca/internal/pdfinspector/chunking"
	"arca/internal/pdfinspector/config"
	"arca/internal/pdfinspector/diagnostics"
	"arca/internal/pdfinspector/enrichment"
	"arca/internal/pdfinspector/firecrawl"
	"arca/internal/pdfinspector/model"
	"arca/internal/pdfinspector/semantic"
)

// Inspector defines the primary interface for PDF inspection.
type Inspector interface {
	InspectPDF(ctx context.Context, r io.Reader) (*model.PDFInspectionResult, error)
}

// PDFInspector orchestrates the document inspection pipeline using injected abstractions.
type PDFInspector struct {
	cfg        *config.Config
	client     firecrawl.Client
	processor  semantic.Processor
	chunker    chunking.Engine
	extractor  assets.Extractor
	aggregator diagnostics.Aggregator
	enricher   enrichment.Enricher
}

// NewPDFInspector constructs a PDFInspector with injected dependencies.
func NewPDFInspector(
	cfg *config.Config,
	client firecrawl.Client,
	processor semantic.Processor,
	chunker chunking.Engine,
	extractor assets.Extractor,
	aggregator diagnostics.Aggregator,
) *PDFInspector {
	return &PDFInspector{
		cfg:        cfg,
		client:     client,
		processor:  processor,
		chunker:    chunker,
		extractor:  extractor,
		aggregator: aggregator,
		enricher:   enrichment.NewEnricher(),
	}
}

// NewPDFInspectorWithEnricher constructs a PDFInspector with explicit Enricher seam injection.
func NewPDFInspectorWithEnricher(
	cfg *config.Config,
	client firecrawl.Client,
	processor semantic.Processor,
	chunker chunking.Engine,
	extractor assets.Extractor,
	aggregator diagnostics.Aggregator,
	enricher enrichment.Enricher,
) *PDFInspector {
	if enricher == nil {
		enricher = enrichment.NewEnricher()
	}
	return &PDFInspector{
		cfg:        cfg,
		client:     client,
		processor:  processor,
		chunker:    chunker,
		extractor:  extractor,
		aggregator: aggregator,
		enricher:   enricher,
	}
}

// InspectPDF executes the complete end-to-end PDF inspection pipeline with fail-fast resiliency checks and diagnostics aggregation.
func (i *PDFInspector) InspectPDF(ctx context.Context, r io.Reader) (*model.PDFInspectionResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	start := time.Now().UnixMilli()

	// 0. Fail-fast PDF validation check
	data, valErr := diagnostics.ValidatePDFStream(r)
	if valErr != nil {
		mappedErr := diagnostics.MapFirecrawlError(valErr)
		diag := i.aggregator.BuildDiagnosticsWithOptions(diagnostics.DiagnosticOptions{
			Status:    model.StatusFailed,
			Errors:    []string{mappedErr.Error()},
			StartTime: start,
		})
		res := model.NewPDFInspectionResult()
		res.Diagnostics = diag
		return res, mappedErr
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 1. Raw extraction via Firecrawl client
	raw, err := i.client.ExtractPDF(ctx, bytes.NewReader(data))
	if err != nil {
		mappedErr := diagnostics.MapFirecrawlError(err)
		diag := i.aggregator.BuildDiagnosticsWithOptions(diagnostics.DiagnosticOptions{
			Status:    model.StatusFailed,
			Errors:    []string{mappedErr.Error()},
			StartTime: start,
		})
		res := model.NewPDFInspectionResult()
		res.Diagnostics = diag
		return res, mappedErr
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	skippedPages := diagnostics.ExtractSkippedPages(raw)
	retryCount := diagnostics.ExtractRetryCount(raw)

	// 2. Semantic tree reconstruction
	tree, err := i.processor.ProcessExtraction(ctx, raw)
	if err != nil {
		mappedErr := diagnostics.MapFirecrawlError(err)
		diag := i.aggregator.BuildDiagnosticsWithOptions(diagnostics.DiagnosticOptions{
			Status:       model.StatusFailed,
			Errors:       []string{mappedErr.Error()},
			SkippedPages: skippedPages,
			RetryCount:   retryCount,
			StartTime:    start,
		})
		res := model.NewPDFInspectionResult()
		res.Diagnostics = diag
		return res, mappedErr
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 3. Hierarchical semantic chunking
	chunks, err := i.chunker.ChunkDocument(ctx, tree, raw.Markdown)
	if err != nil {
		mappedErr := diagnostics.MapFirecrawlError(err)
		diag := i.aggregator.BuildDiagnosticsWithOptions(diagnostics.DiagnosticOptions{
			Status:       model.StatusFailed,
			Errors:       []string{mappedErr.Error()},
			SkippedPages: skippedPages,
			RetryCount:   retryCount,
			StartTime:    start,
		})
		res := model.NewPDFInspectionResult()
		res.Diagnostics = diag
		return res, mappedErr
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	docContent := buildDocumentContent(raw)

	// 4. Asset extraction
	extractedAssets, err := i.extractor.ExtractAssetsWithContext(ctx, tree, docContent, chunks)
	if err != nil {
		mappedErr := diagnostics.MapFirecrawlError(err)
		diag := i.aggregator.BuildDiagnosticsWithOptions(diagnostics.DiagnosticOptions{
			Status:       model.StatusFailed,
			Errors:       []string{mappedErr.Error()},
			SkippedPages: skippedPages,
			RetryCount:   retryCount,
			StartTime:    start,
		})
		res := model.NewPDFInspectionResult()
		res.Diagnostics = diag
		return res, mappedErr
	}

	var warnings []string
	if wc, ok := i.processor.(interface{ Warnings() []string }); ok {
		warnings = append(warnings, wc.Warnings()...)
	}
	if wc, ok := i.chunker.(interface{ Warnings() []string }); ok {
		warnings = append(warnings, wc.Warnings()...)
	}
	if extractedAssets != nil && len(extractedAssets.Warnings) > 0 {
		for _, w := range extractedAssets.Warnings {
			warnings = append(warnings, w.Message)
		}
	}

	docMeta := buildDocumentMetadata(raw, tree, chunks, docContent.PageMap)

	// 5. Semantic Metadata Enrichment Layer (ADR 0007 via Deep Enricher Seam)
	enrichReport := i.enricher.Enrich(ctx, &enrichment.EnrichmentInput{
		Metadata: &docMeta,
		Tree:     tree,
		PageMap:  docContent.PageMap,
		Chunks:   chunks,
	})
	if enrichReport != nil && len(enrichReport.Warnings) > 0 {
		warnings = append(warnings, enrichReport.Warnings...)
	}

	diag := i.aggregator.BuildDiagnosticsWithOptions(diagnostics.DiagnosticOptions{
		Status:       model.StatusSuccess,
		Warnings:     warnings,
		SkippedPages: skippedPages,
		RetryCount:   retryCount,
		StartTime:    start,
	})

	result := model.NewPDFInspectionResult()
	result.Document = docMeta
	result.SemanticTree = *tree
	result.Content = *docContent
	result.Chunks = chunks
	result.Assets = *extractedAssets
	result.Diagnostics = diag

	return result, nil
}

// buildDocumentMetadata extracts administrative and structural metadata from extraction metadata and semantic structures.
func buildDocumentMetadata(raw *model.RawExtractionResult, tree *model.SemanticTree, chunks []model.KnowledgeChunk, pageMap []model.PageMap) model.DocumentMetadata {
	meta := model.DocumentMetadata{
		Fonts:      []string{},
		Searchable: true,
	}

	if raw != nil && raw.Metadata != nil {
		if val, ok := raw.Metadata["title"].(string); ok && val != "" {
			meta.Title = val
		}
		if val, ok := raw.Metadata["author"].(string); ok && val != "" {
			meta.Author = val
		}
		if val, ok := raw.Metadata["creator"].(string); ok && val != "" {
			meta.Creator = val
		}
		if val, ok := raw.Metadata["producer"].(string); ok && val != "" {
			meta.Producer = val
		}
		if val, ok := raw.Metadata["page_dimensions"].(string); ok && val != "" {
			meta.PageDimensions = val
		}
		if val, ok := raw.Metadata["pageDimensions"].(string); ok && val != "" {
			meta.PageDimensions = val
		}
		if val, ok := raw.Metadata["page_count"]; ok {
			switch v := val.(type) {
			case float64:
				meta.PageCount = int(v)
			case int:
				meta.PageCount = v
			}
		}
		if val, ok := raw.Metadata["fonts"].([]interface{}); ok {
			for _, font := range val {
				if fStr, ok := font.(string); ok && fStr != "" {
					meta.Fonts = append(meta.Fonts, fStr)
				}
			}
		}
		if val, ok := raw.Metadata["encrypted"].(bool); ok {
			meta.Encrypted = val
		}
		if val, ok := raw.Metadata["searchable"].(bool); ok {
			meta.Searchable = val
		}
		if val, ok := raw.Metadata["pdf_type"].(string); ok && val != "" {
			meta.PDFType = val
		}
		if val, ok := raw.Metadata["pdfType"].(string); ok && val != "" {
			meta.PDFType = val
		}
	}

	// Fallback title to first heading in semantic tree
	if meta.Title == "" && tree != nil && len(tree.RootNodes) > 0 {
		meta.Title = tree.RootNodes[0].Heading
	}
	if meta.Title == "" {
		meta.Title = "Untitled Document"
	}

	// Fallback pageCount if missing or zero
	if meta.PageCount <= 0 {
		maxPage := len(pageMap)
		for _, ch := range chunks {
			for _, p := range ch.PageNumbers {
				if p > maxPage {
					maxPage = p
				}
			}
		}
		if maxPage <= 0 {
			maxPage = 1
		}
		meta.PageCount = maxPage
	}

	return meta
}

// buildDocumentContent constructs DocumentContent and per-page PageMap array.
func buildDocumentContent(raw *model.RawExtractionResult) *model.DocumentContent {
	content := &model.DocumentContent{
		Markdown: raw.Markdown,
		PageMap:  []model.PageMap{},
	}

	if raw != nil && raw.JSONLayout != nil {
		if pagesVal, ok := raw.JSONLayout["pages"].([]interface{}); ok && len(pagesVal) > 0 {
			for idx, pItem := range pagesVal {
				if pMap, ok := pItem.(map[string]interface{}); ok {
					pageNum := idx + 1
					if pNumVal, ok := pMap["page_number"]; ok {
						switch v := pNumVal.(type) {
						case float64:
							pageNum = int(v)
						case int:
							pageNum = v
						}
					}
					pageMd := ""
					if mdVal, ok := pMap["markdown"].(string); ok {
						pageMd = mdVal
					} else if txtVal, ok := pMap["text"].(string); ok {
						pageMd = txtVal
					}
					content.PageMap = append(content.PageMap, model.PageMap{
						PageNumber: pageNum,
						Markdown:   pageMd,
					})
				}
			}
		}
	}

	if len(content.PageMap) == 0 {
		content.PageMap = []model.PageMap{
			{PageNumber: 1, Markdown: raw.Markdown},
		}
	}

	return content
}
