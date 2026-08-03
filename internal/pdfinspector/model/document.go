package model

import "time"

// DocumentMetadata represents administrative, technical, and structural metadata extracted from a PDF document.
type DocumentMetadata struct {
	DocumentID       string    `json:"documentID,omitempty"`
	Title            string    `json:"title,omitempty"`
	Author           string    `json:"author,omitempty"`
	Creator          string    `json:"creator,omitempty"`
	Producer         string    `json:"producer,omitempty"`
	CreationDate     time.Time `json:"creationDate,omitempty"`
	ModificationDate time.Time `json:"modificationDate,omitempty"`
	PageCount        int       `json:"pageCount"`
	PageDimensions   string    `json:"pageDimensions,omitempty"`
	Fonts            []string  `json:"fonts,omitempty"`
	Encrypted        bool      `json:"encrypted"`
	Searchable       bool      `json:"searchable"`
	PDFType          string    `json:"pdfType,omitempty"`
	Language         string    `json:"language,omitempty"`
	Keywords         []Keyword `json:"keywords,omitempty"`
	Entities         []Entity   `json:"entities,omitempty"`
	Concepts         []Concept  `json:"concepts,omitempty"`
	Relations        []Relation `json:"relations,omitempty"`
	Summary          *Summary   `json:"summary,omitempty"`
}

// PageMap maps an individual PDF page number to its extracted raw Markdown text.
type PageMap struct {
	PageNumber int    `json:"pageNumber"`
	Markdown   string `json:"markdown"`
}

// DocumentContent encapsulates extracted complete text and per-page layout maps.
type DocumentContent struct {
	Markdown string    `json:"markdown"`
	PageMap  []PageMap `json:"pageMap"`
}
