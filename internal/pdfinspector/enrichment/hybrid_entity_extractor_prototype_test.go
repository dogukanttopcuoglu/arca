// PROTOTYPE — HybridEntityExtractor unit test
// THROWAWAY CODE — on branch: prototype/gliner-entity-extraction

package enrichment_test

import (
	"context"
	"testing"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

// stubExtractor is a test double that returns a fixed set of mentions.
type stubExtractor struct {
	mentions []pdfmodel.EntityMention
}

func (s *stubExtractor) ExtractEntities(_ context.Context, _ enrichment.EntityInput) ([]pdfmodel.EntityMention, error) {
	return s.mentions, nil
}

func TestHybridEntityExtractor_MergesRuleBasedAndGLiNER(t *testing.T) {
	ctx := context.Background()

	// RuleBased finds: Rick Rubin, Def Jam Recordings, New York
	ruleMentions := []pdfmodel.EntityMention{
		{Text: "Rick Rubin", Type: pdfmodel.EntityTypePerson, ChunkID: "chunk-1", Confidence: 0.90},
		{Text: "Def Jam Recordings", Type: pdfmodel.EntityTypeOrganization, ChunkID: "chunk-1", Confidence: 0.85},
		{Text: "New York", Type: pdfmodel.EntityTypeLocation, ChunkID: "chunk-1", Confidence: 0.88},
	}

	// GLiNER additionally finds: Russell Simmons (the recall gap case)
	// Also re-finds Rick Rubin with lower confidence (should not override rule's higher conf)
	glinerMentions := []pdfmodel.EntityMention{
		{Text: "Russell Simmons", Type: pdfmodel.EntityTypePerson, ChunkID: "chunk-1", Confidence: 0.94},
		{Text: "Rick Rubin", Type: pdfmodel.EntityTypePerson, ChunkID: "chunk-1", Confidence: 0.80}, // lower conf
	}

	hybrid := enrichment.NewHybridEntityExtractor(
		&stubExtractor{mentions: ruleMentions},
		&stubExtractor{mentions: glinerMentions},
	)

	input := enrichment.EntityInput{
		Chunks:   []pdfmodel.KnowledgeChunk{{ChunkID: "chunk-1", ContentMarkdown: "Rick Rubin founded Def Jam Recordings in New York alongside Russell Simmons."}},
		Language: "en",
	}

	mentions, err := hybrid.ExtractEntities(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Must find all 4 entities
	textSet := make(map[string]pdfmodel.EntityMention)
	for _, m := range mentions {
		textSet[m.Text] = m
	}

	expected := []string{"Rick Rubin", "Def Jam Recordings", "New York", "Russell Simmons"}
	for _, name := range expected {
		if _, ok := textSet[name]; !ok {
			t.Errorf("BENCHMARK FAILURE: expected entity %q not found in hybrid output", name)
		}
	}

	// Rule confidence must win over GLiNER for Rick Rubin (0.90 > 0.80)
	if m, ok := textSet["Rick Rubin"]; ok {
		if m.Confidence != 0.90 {
			t.Errorf("expected Rick Rubin confidence=0.90 (rule wins), got %.2f", m.Confidence)
		}
	}

	// GLiNER-only entity Russell Simmons must be present
	if m, ok := textSet["Russell Simmons"]; ok {
		if m.Type != pdfmodel.EntityTypePerson {
			t.Errorf("expected Russell Simmons type=person, got %q", m.Type)
		}
		if m.Confidence != 0.94 {
			t.Errorf("expected Russell Simmons confidence=0.94, got %.2f", m.Confidence)
		}
	}

	t.Logf("BENCHMARK RESULT: Hybrid extracted %d entities: %v", len(mentions), expected)
}

func TestHybridEntityExtractor_GracefullyHandlesGLiNERFailure(t *testing.T) {
	ctx := context.Background()

	ruleMentions := []pdfmodel.EntityMention{
		{Text: "Rick Rubin", Type: pdfmodel.EntityTypePerson, ChunkID: "chunk-1", Confidence: 0.90},
	}

	// GLiNER returns error (service down)
	failingGLiNER := &stubFailExtractor{}

	hybrid := enrichment.NewHybridEntityExtractor(
		&stubExtractor{mentions: ruleMentions},
		failingGLiNER,
	)

	input := enrichment.EntityInput{
		Chunks:   []pdfmodel.KnowledgeChunk{{ChunkID: "chunk-1", ContentMarkdown: "Rick Rubin."}},
		Language: "en",
	}

	mentions, err := hybrid.ExtractEntities(ctx, input)
	if err != nil {
		t.Fatalf("hybrid should not propagate secondary failure: %v", err)
	}
	if len(mentions) == 0 {
		t.Error("expected at least rule-based mentions when GLiNER is down")
	}
}

type stubFailExtractor struct{}

func (s *stubFailExtractor) ExtractEntities(_ context.Context, _ enrichment.EntityInput) ([]pdfmodel.EntityMention, error) {
	return nil, context.DeadlineExceeded
}
