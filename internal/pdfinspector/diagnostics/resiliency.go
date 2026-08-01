package diagnostics

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"arca/internal/pdfinspector/model"
)

// MaxHeaderCheckBytes defines how far into the PDF stream we search for %PDF- signature.
const MaxHeaderCheckBytes = 1024

// ValidatePDFStream reads PDF data from r, checks for minimum PDF structure and encryption,
// returning the buffered bytes and any fail-fast error.
func ValidatePDFStream(r io.Reader) ([]byte, error) {
	if r == nil {
		return nil, model.ErrInvalidDocument
	}

	if seeker, ok := r.(io.Seeker); ok {
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("failed to seek PDF stream: %w", err)
		}
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read PDF stream: %v", model.ErrInvalidDocument, err)
	}

	if len(data) == 0 {
		return nil, model.ErrInvalidDocument
	}

	// Check for %PDF- header signature in first MaxHeaderCheckBytes
	checkLimit := len(data)
	if checkLimit > MaxHeaderCheckBytes {
		checkLimit = MaxHeaderCheckBytes
	}

	headerIdx := bytes.Index(data[:checkLimit], []byte("%PDF-"))
	if headerIdx == -1 {
		return nil, model.ErrInvalidDocument
	}

	// Check for encryption markers in the document
	if containsEncryptionMarker(data) {
		return nil, model.ErrEncryptedDocument
	}

	return data, nil
}

// containsEncryptionMarker detects standard PDF encryption dictionary indicators.
func containsEncryptionMarker(data []byte) bool {
	if bytes.Contains(data, []byte("/Encrypt")) {
		if bytes.Contains(data, []byte("/Filter")) || bytes.Contains(data, []byte("/Standard")) || bytes.Contains(data, []byte("/P -")) || bytes.Contains(data, []byte("/Encrypt ")) {
			return true
		}
	}
	return false
}

// MapFirecrawlError maps HTTP client, service, or API error messages into canonical model errors.
func MapFirecrawlError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, model.ErrEncryptedDocument) || errors.Is(err, model.ErrInvalidDocument) {
		return err
	}

	errStr := strings.ToLower(err.Error())

	if strings.Contains(errStr, "encrypted") || strings.Contains(errStr, "password") || strings.Contains(errStr, "encrypted_document") {
		return model.ErrEncryptedDocument
	}

	if strings.Contains(errStr, "invalid pdf") || strings.Contains(errStr, "invalid_document") || strings.Contains(errStr, "corrupted") || strings.Contains(errStr, "malformed") || strings.Contains(errStr, "not a pdf") || strings.Contains(errStr, "header not found") {
		return model.ErrInvalidDocument
	}

	return err
}

// ExtractSkippedPages extracts skipped or failed page numbers from RawExtractionResult layout/metadata.
func ExtractSkippedPages(raw *model.RawExtractionResult) []int {
	if raw == nil {
		return []int{}
	}

	skippedMap := make(map[int]bool)

	extractFromMap := func(m map[string]interface{}, key string) {
		val, ok := m[key]
		if !ok || val == nil {
			return
		}
		if slice, ok := val.([]interface{}); ok {
			for _, item := range slice {
				switch v := item.(type) {
				case float64:
					if int(v) > 0 {
						skippedMap[int(v)] = true
					}
				case int:
					if v > 0 {
						skippedMap[v] = true
					}
				}
			}
		}
	}

	if raw.Metadata != nil {
		extractFromMap(raw.Metadata, "skipped_pages")
		extractFromMap(raw.Metadata, "failed_pages")
	}

	if raw.JSONLayout != nil {
		extractFromMap(raw.JSONLayout, "skipped_pages")
		extractFromMap(raw.JSONLayout, "failed_pages")
	}

	result := make([]int, 0, len(skippedMap))
	for page := range skippedMap {
		result = append(result, page)
	}

	sort.Ints(result)
	return result
}

// ExtractRetryCount extracts retry attempts recorded in RawExtractionResult metadata.
func ExtractRetryCount(raw *model.RawExtractionResult) int {
	if raw == nil || raw.Metadata == nil {
		return 0
	}

	if val, ok := raw.Metadata["retry_count"]; ok {
		switch v := val.(type) {
		case float64:
			return int(v)
		case int:
			return v
		}
	}
	return 0
}
