package enrichment_test

import (
	"context"
	"testing"

	"arca/internal/pdfinspector/enrichment"
	pdfmodel "arca/internal/pdfinspector/model"
)

func TestHybridEntityExtractor_MergesRuleBasedAndGLiNER(t *testing.T) {
	ctx := context.Background()

	// RuleBased finds: Rick Rubin, Def Jam Recordings, New York
	ruleMentions := []pdfmodel.EntityMention{
		{Text: "Rick Rubin", Type: pdfmodel.EntityTypePerson, ChunkID: "chunk-1", Confidence: 0.90},
		{Text: "Def Jam Recordings", Type: pdfmodel.EntityTypeOrganization, ChunkID: "chunk-1", Confidence: 0.85},
		{Text: "New York", Type: pdfmodel.EntityTypeLocation, ChunkID: "chunk-1", Confidence: 0.88},
	}

	// GLiNER additionally finds Russell Simmons (the recall gap case)
	// and re-finds Rick Rubin with lower confidence (rule must win)
	glinerMentions := []pdfmodel.EntityMention{
		{Text: "Russell Simmons", Type: pdfmodel.EntityTypePerson, ChunkID: "chunk-1", Confidence: 0.94},
		{Text: "Rick Rubin", Type: pdfmodel.EntityTypePerson, ChunkID: "chunk-1", Confidence: 0.80},
	}

	hybrid := enrichment.NewHybridEntityExtractor(
		&stubPrimaryExtractor{mentions: ruleMentions},
		&stubPrimaryExtractor{mentions: glinerMentions},
	)

	input := enrichment.EntityInput{
		Chunks:   []pdfmodel.KnowledgeChunk{{ChunkID: "chunk-1", ContentMarkdown: "Rick Rubin founded Def Jam Recordings in New York alongside Russell Simmons."}},
		Language: "en",
	}

	mentions, err := hybrid.ExtractEntities(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	textSet := make(map[string]pdfmodel.EntityMention)
	for _, m := range mentions {
		textSet[m.Text] = m
	}

	// All 4 must be present
	expected := []string{"Rick Rubin", "Def Jam Recordings", "New York", "Russell Simmons"}
	for _, name := range expected {
		if _, ok := textSet[name]; !ok {
			t.Errorf("expected entity %q not found in hybrid output", name)
		}
	}

	// Rule confidence must win for Rick Rubin (0.90 > 0.80)
	if m, ok := textSet["Rick Rubin"]; ok {
		if m.Confidence != 0.90 {
			t.Errorf("expected Rick Rubin confidence=0.90 (rule wins), got %.2f", m.Confidence)
		}
	}

	// GLiNER-only: Russell Simmons must be present with GLiNER confidence
	if m, ok := textSet["Russell Simmons"]; ok {
		if m.Type != pdfmodel.EntityTypePerson {
			t.Errorf("expected Russell Simmons type=person, got %q", m.Type)
		}
		if m.Confidence != 0.94 {
			t.Errorf("expected Russell Simmons confidence=0.94, got %.2f", m.Confidence)
		}
	}
}

func TestHybridEntityExtractor_GracefullyHandlesSecondaryFailure(t *testing.T) {
	ctx := context.Background()

	ruleMentions := []pdfmodel.EntityMention{
		{Text: "Rick Rubin", Type: pdfmodel.EntityTypePerson, ChunkID: "chunk-1", Confidence: 0.90},
	}

	hybrid := enrichment.NewHybridEntityExtractor(
		&stubPrimaryExtractor{mentions: ruleMentions},
		&stubFailingExtractor{},
	)

	input := enrichment.EntityInput{
		Chunks:   []pdfmodel.KnowledgeChunk{{ChunkID: "chunk-1", ContentMarkdown: "Rick Rubin."}},
		Language: "en",
	}

	mentions, err := hybrid.ExtractEntities(ctx, input)
	if err != nil {
		t.Fatalf("hybrid must not propagate secondary failure: %v", err)
	}
	if len(mentions) == 0 {
		t.Error("expected primary mentions when secondary is down")
	}
}

func TestHybridEntityExtractor_HigherConfidenceWinsOnCollision(t *testing.T) {
	ctx := context.Background()

	primary := []pdfmodel.EntityMention{
		{Text: "Def Jam Recordings", Type: pdfmodel.EntityTypeOrganization, ChunkID: "c1", Confidence: 0.85},
	}
	secondary := []pdfmodel.EntityMention{
		{Text: "Def Jam Recordings", Type: pdfmodel.EntityTypeOrganization, ChunkID: "c1", Confidence: 0.97},
	}

	hybrid := enrichment.NewHybridEntityExtractor(
		&stubPrimaryExtractor{mentions: primary},
		&stubPrimaryExtractor{mentions: secondary},
	)

	input := enrichment.EntityInput{Chunks: []pdfmodel.KnowledgeChunk{{ChunkID: "c1"}}}
	mentions, _ := hybrid.ExtractEntities(ctx, input)

	if len(mentions) != 1 {
		t.Fatalf("expected 1 merged entity, got %d", len(mentions))
	}
	if mentions[0].Confidence != 0.97 {
		t.Errorf("expected GLiNER confidence 0.97 to win, got %.2f", mentions[0].Confidence)
	}
}

// --- Test doubles ---

type stubPrimaryExtractor struct {
	mentions []pdfmodel.EntityMention
}

func (s *stubPrimaryExtractor) ExtractEntities(_ context.Context, _ enrichment.EntityInput) ([]pdfmodel.EntityMention, error) {
	return s.mentions, nil
}

type stubFailingExtractor struct{}

func (s *stubFailingExtractor) ExtractEntities(_ context.Context, _ enrichment.EntityInput) ([]pdfmodel.EntityMention, error) {
	return nil, context.DeadlineExceeded
}
