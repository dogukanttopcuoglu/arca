package enrichment

import (
	"context"
	"strings"
)

const (
	CapabilityLanguage Capability = "language"
)

// LanguageDetectionPass implements EnricherPass for detecting document language (e.g. "tr", "en").
type LanguageDetectionPass struct{}

// NewLanguageDetectionPass constructs a LanguageDetectionPass instance.
func NewLanguageDetectionPass() *LanguageDetectionPass {
	return &LanguageDetectionPass{}
}

func (p *LanguageDetectionPass) Name() string           { return "LanguageDetectionPass" }
func (p *LanguageDetectionPass) Requires() []Capability { return []Capability{CapabilityRawMetadata} }
func (p *LanguageDetectionPass) Provides() []Capability { return []Capability{CapabilityLanguage} }

func (p *LanguageDetectionPass) Execute(ctx context.Context, input *EnrichmentInput) ([]string, error) {
	if input == nil || input.Metadata == nil {
		return nil, nil
	}

	sampleText := ""
	for _, pm := range input.PageMap {
		if pm.PageNumber <= 5 {
			sampleText += " " + pm.Markdown
		}
	}
	if sampleText == "" {
		for _, ch := range input.Chunks {
			sampleText += " " + ch.ContentMarkdown
		}
	}

	lang := DetectLanguage(sampleText)
	input.Metadata.Language = lang

	return nil, nil
}

// DetectLanguage inspects sample text to determine ISO 639-1 language code ("tr", "en").
func DetectLanguage(text string) string {
	lower := strings.ToLower(text)

	// Turkish specific characters & stopword heuristics
	trChars := []string{"ç", "ğ", "ı", "ö", "ş", "ü", "veya", "için", "bir", "olarak", "hakkında", "bu"}
	trCount := 0
	for _, char := range trChars {
		if strings.Contains(lower, char) {
			trCount++
		}
	}

	if trCount >= 2 {
		return "tr"
	}

	return "en"
}
