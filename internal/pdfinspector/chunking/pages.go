package chunking

import (
	"strings"

	"arca/internal/pdfinspector/model"
)

// resolveBlockPages assigns authoritative page numbers to blocks from the document
// PageMap (derived from the extraction service's json_layout.pages).
//
// The bundled Firecrawl service never emits inline page markers (<!-- page:N -->,
// \f, [page N]) in its markdown, so the parser defaults every block to page 1.
// json_layout.pages is the authoritative page layout, so when a PageMap is present
// it overrides the marker-derived default entirely.
//
// Matching is content-based with a forward cursor: blocks appear in reading order,
// so pages are scanned monotonically from the previously resolved page. This keeps
// resolution deterministic even when the same text appears on multiple pages.
func resolveBlockPages(blocks []SemanticBlock, pageMap []model.PageMap) {
	if len(pageMap) == 0 {
		return
	}

	pages := make([]normPage, 0, len(pageMap))
	for _, pm := range pageMap {
		t := normalizePageText(pm.Markdown)
		if t == "" {
			continue
		}
		pages = append(pages, normPage{num: pm.PageNumber, text: t})
	}
	if len(pages) == 0 {
		return
	}

	cursor := 0
	for i := range blocks {
		text := normalizePageText(blocks[i].Markdown)
		if text == "" {
			continue
		}
		pg := findBlockPage(pages, cursor, text)
		if pg == 0 {
			continue
		}
		blocks[i].PageNumbers = []int{pg}

		// Propagate the resolved page to citations extracted during parsing.
		for j := range blocks[i].Citations {
			blocks[i].Citations[j].PageNumber = pg
		}

		for j := cursor; j < len(pages); j++ {
			if pages[j].num == pg {
				cursor = j
				break
			}
		}
	}
}

// normPage is a pre-normalized PageMap entry used for O(1) content matching.
type normPage struct {
	num  int
	text string
}

// findBlockPage locates the first page at or after start whose markdown contains
// text. It prefers an exact (whitespace-normalized) match, then progressively
// shortens the signature to locate blocks that straddle page boundaries.
func findBlockPage(pages []normPage, start int, text string) int {
	for j := start; j < len(pages); j++ {
		if strings.Contains(pages[j].text, text) {
			return pages[j].num
		}
	}

	runes := []rune(text)
	step := len(runes) / 4
	if step < 1 {
		step = 1
	}
	for cut := step; cut < len(runes); cut += step {
		prefix := string(runes[:len(runes)-cut])
		for j := start; j < len(pages); j++ {
			if strings.Contains(pages[j].text, prefix) {
				return pages[j].num
			}
		}
	}
	return 0
}

// normalizePageText collapses all whitespace for robust substring matching while
// preserving case (page markdown and block content must match exactly).
func normalizePageText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
