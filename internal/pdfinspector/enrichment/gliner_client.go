package enrichment

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	pdfmodel "arca/internal/pdfinspector/model"
)

// GLiNEREntityExtractor implements EntityExtractor via HTTP REST connection to a GLiNER NER microservice with fallback.
type GLiNEREntityExtractor struct {
	endpoint string
	client   *http.Client
	fallback EntityExtractor
}

type glinerRequest struct {
	Text   string   `json:"text"`
	Labels []string `json:"labels"`
}

type glinerEntityResponse struct {
	Text       string  `json:"text"`
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
}

// NewGLiNEREntityExtractor constructs a GLiNEREntityExtractor instance with fallback to RuleBasedEntityExtractor.
func NewGLiNEREntityExtractor(endpoint string, timeout time.Duration) *GLiNEREntityExtractor {
	if endpoint == "" {
		endpoint = "http://localhost:8088/extract-entities"
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &GLiNEREntityExtractor{
		endpoint: endpoint,
		client:   &http.Client{Timeout: timeout},
		fallback: NewRuleBasedEntityExtractor(),
	}
}

// ExtractEntities sends chunk text to GLiNER service, falling back to rule-based extraction on network failure.
func (g *GLiNEREntityExtractor) ExtractEntities(ctx context.Context, input EntityInput) ([]pdfmodel.EntityMention, error) {
	if len(input.Chunks) == 0 {
		return []pdfmodel.EntityMention{}, nil
	}

	var allMentions []pdfmodel.EntityMention
	labels := input.Labels
	if len(labels) == 0 {
		labels = []string{"person", "organization", "location", "product", "work of art"}
	}

	for _, ch := range input.Chunks {
		if ch.ContentMarkdown == "" {
			continue
		}

		mentions, err := g.extractFromEndpoint(ctx, ch.ChunkID, ch.ContentMarkdown, labels)
		if err != nil || len(mentions) == 0 {
			// Resilient Fallback to Rule-Based Extractor for this chunk
			fallbackMentions, _ := g.fallback.ExtractEntities(ctx, EntityInput{
				Chunks:   []pdfmodel.KnowledgeChunk{ch},
				Language: input.Language,
				Labels:   labels,
			})
			allMentions = append(allMentions, fallbackMentions...)
		} else {
			allMentions = append(allMentions, mentions...)
		}
	}

	return allMentions, nil
}

func (g *GLiNEREntityExtractor) extractFromEndpoint(ctx context.Context, chunkID, text string, labels []string) ([]pdfmodel.EntityMention, error) {
	reqBody, err := json.Marshal(glinerRequest{Text: text, Labels: labels})
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, err
	}

	var rawResp []glinerEntityResponse
	if err := json.NewDecoder(resp.Body).Decode(&rawResp); err != nil {
		return nil, err
	}

	var mentions []pdfmodel.EntityMention
	for _, item := range rawResp {
		mentions = append(mentions, pdfmodel.EntityMention{
			Text:       item.Text,
			Type:       pdfmodel.EntityType(item.Label),
			ChunkID:    chunkID,
			Confidence: item.Confidence,
		})
	}

	return mentions, nil
}
