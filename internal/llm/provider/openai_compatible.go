package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	qaprompt "arca/internal/qa/prompt"
	"github.com/valyala/fasthttp"
)

// openAICompletionsEndpoint is the OpenAI-compatible chat completions path
// appended to the configured base URL (which already includes any /v1 prefix).
const openAICompletionsEndpoint = "/chat/completions"

// OpenAICompatibleProvider is a provider-neutral LLMProvider adapter speaking
// the OpenAI chat/completions protocol against any compatible gateway. It
// contains no provider-specific logic or identifiers: base URL, bearer
// credential, model identifier, and provider label are pure configuration.
type OpenAICompatibleProvider struct {
	baseURL       string
	apiKey        string
	model         string
	providerLabel string
	client        *fasthttp.Client
	timeout       time.Duration
}

// OpenAICompatibleOption configures an OpenAICompatibleProvider instance.
type OpenAICompatibleOption func(*OpenAICompatibleProvider)

// WithOpenAICompatibleTimeout overrides the request timeout.
func WithOpenAICompatibleTimeout(timeout time.Duration) OpenAICompatibleOption {
	return func(p *OpenAICompatibleProvider) {
		if timeout > 0 {
			p.timeout = timeout
		}
	}
}

// NewOpenAICompatibleProvider constructs an OpenAI-compatible LLMProvider
// adapter. baseURL is the gateway root (defaults to the current deployment
// target per ADR-0026); apiKey is the bearer credential (empty for keyless
// endpoints); model is forwarded verbatim and never validated, aliased, or
// assumed — an empty model fails generation with a clear error; providerLabel
// is observability-only and only surfaces in responses.
func NewOpenAICompatibleProvider(baseURL, apiKey, model, providerLabel string, opts ...OpenAICompatibleOption) *OpenAICompatibleProvider {
	p := &OpenAICompatibleProvider{
		baseURL:       "https://agentrouter.org/v1",
		providerLabel: providerLabel,
		client:        &fasthttp.Client{},
		timeout:       60 * time.Second,
	}

	if baseURL != "" {
		p.baseURL = strings.TrimRight(baseURL, "/")
	}
	p.apiKey = apiKey
	p.model = model
	if providerLabel == "" {
		p.providerLabel = "openai-compatible"
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Generate executes a synchronous chat completion and maps the response
// content and token usage onto the vendor-agnostic LLMResponse.
func (p *OpenAICompatibleProvider) Generate(ctx context.Context, prompt qaprompt.PromptMessage) (*LLMResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.model == "" {
		return nil, fmt.Errorf("model identifier is not configured")
	}

	body, err := json.Marshal(p.buildRequest(prompt))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal chat completion request: %w", err)
	}

	respBody, err := p.doPost(ctx, openAICompletionsEndpoint, body)
	if err != nil {
		return nil, err
	}

	var out openAICompletionResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("failed to decode chat completion response: %w", err)
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == "" {
		return nil, fmt.Errorf("chat completion response contained no assistant content")
	}

	model := out.Model
	if model == "" {
		model = p.model
	}

	return &LLMResponse{
		Content:  out.Choices[0].Message.Content,
		Model:    model,
		Provider: p.providerLabel,
		TokenUsage: LLMUsage{
			PromptTokens:     out.Usage.PromptTokens,
			CompletionTokens: out.Usage.CompletionTokens,
			TotalTokens:      out.Usage.TotalTokens,
		},
	}, nil
}

// Stream is a convenience wrapper over Generate: the adapter performs a single
// non-streamed completion and emits it as one token chunk. Real SSE streaming
// is not part of M2; nothing in the M2 pipeline depends on it.
func (p *OpenAICompatibleProvider) Stream(ctx context.Context, prompt qaprompt.PromptMessage) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 2)

	go func() {
		defer close(ch)
		resp, err := p.Generate(ctx, prompt)
		if err != nil {
			ch <- StreamChunk{Error: err}
			return
		}
		ch <- StreamChunk{Content: resp.Content}
		ch <- StreamChunk{Done: true}
	}()

	return ch, nil
}

// Capabilities reports the provider operational bounds.
func (p *OpenAICompatibleProvider) Capabilities() ModelCapabilities {
	return ModelCapabilities{
		SupportsSystemMessage: true,
		SupportsStreaming:     true,
		ContextWindow:         128000,
	}
}

// buildRequest maps the vendor-agnostic PromptMessage onto the OpenAI
// chat/completions request shape. The model identifier is forwarded verbatim.
// Zero-valued options are omitted rather than sent as explicit zeros.
func (p *OpenAICompatibleProvider) buildRequest(prompt qaprompt.PromptMessage) map[string]interface{} {
	messages := make([]map[string]string, 0, len(prompt.Messages)+1)
	if strings.TrimSpace(prompt.System) != "" {
		messages = append(messages, map[string]string{"role": "system", "content": prompt.System})
	}
	for _, msg := range prompt.Messages {
		messages = append(messages, map[string]string{"role": msg.Role, "content": msg.Content})
	}

	req := map[string]interface{}{
		"model":    p.model,
		"messages": messages,
	}
	if prompt.Options.Temperature != 0 {
		req["temperature"] = prompt.Options.Temperature
	}
	if prompt.Options.MaxTokens > 0 {
		req["max_tokens"] = prompt.Options.MaxTokens
	}
	return req
}

// doPost performs a JSON POST to the given endpoint and returns the response
// body, mapping non-200 responses to a typed OpenAIAPIError.
func (p *OpenAICompatibleProvider) doPost(ctx context.Context, endpoint string, body []byte) ([]byte, error) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer func() {
		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)
	}()

	req.SetRequestURI(p.baseURL + endpoint)
	req.Header.SetMethod(fasthttp.MethodPost)
	req.Header.SetContentType("application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	req.SetBody(body)

	if err := p.client.DoTimeout(req, resp, p.timeout); err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		return nil, fmt.Errorf("chat completion request failed: %w", err)
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		return nil, &OpenAIAPIError{
			StatusCode: resp.StatusCode(),
			Message:    strings.TrimSpace(string(resp.Body())),
		}
	}

	// Copy the body out before ReleaseResponse: the slice aliases the pooled
	// response buffer, which a concurrent request may reuse and overwrite.
	bodyCopy := append([]byte(nil), resp.Body()...)
	return bodyCopy, nil
}

// OpenAIAPIError is the typed error returned for non-200 gateway responses.
type OpenAIAPIError struct {
	StatusCode int
	Message    string
}

// Error implements the error interface.
func (e *OpenAIAPIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("openai-compatible endpoint returned status %d", e.StatusCode)
	}
	return fmt.Sprintf("openai-compatible endpoint returned status %d: %s", e.StatusCode, e.Message)
}

// openAICompletionResponse models the chat/completions response fields the
// adapter consumes.
type openAICompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}
