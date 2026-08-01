package firecrawl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"arca/internal/pdfinspector/model"
	"github.com/valyala/fasthttp"
)

// ErrServiceUnavailable indicates that the Firecrawl extraction service is unreachable or returning server errors.
var ErrServiceUnavailable = errors.New("SERVICE_UNAVAILABLE: firecrawl service is unavailable")

// Client defines the interface for interacting with the Firecrawl PDF Extraction HTTP service.
type Client interface {
	ExtractPDF(ctx context.Context, r io.Reader) (*model.RawExtractionResult, error)
}

// HTTPClient implements Client via fasthttp calls with retry and backoff logic.
type HTTPClient struct {
	baseURL           string
	endpoint          string
	client            *fasthttp.Client
	timeout           time.Duration
	maxRetries        int
	initialBackoff    time.Duration
	backoffMultiplier float64
	sleepFunc         func(time.Duration)
}

// Option configures functional options for HTTPClient.
type Option func(*HTTPClient)

// WithBaseURL overrides the base URL.
func WithBaseURL(baseURL string) Option {
	return func(c *HTTPClient) {
		c.baseURL = baseURL
	}
}

// WithEndpoint overrides the API endpoint path.
func WithEndpoint(endpoint string) Option {
	return func(c *HTTPClient) {
		c.endpoint = endpoint
	}
}

// WithClient sets a custom fasthttp.Client.
func WithClient(client *fasthttp.Client) Option {
	return func(c *HTTPClient) {
		c.client = client
	}
}

// WithTimeout sets the request timeout for individual HTTP calls.
func WithTimeout(timeout time.Duration) Option {
	return func(c *HTTPClient) {
		c.timeout = timeout
	}
}

// WithMaxRetries sets the maximum number of retry attempts.
func WithMaxRetries(maxRetries int) Option {
	return func(c *HTTPClient) {
		c.maxRetries = maxRetries
	}
}

// WithInitialBackoff sets the starting duration for exponential backoff.
func WithInitialBackoff(backoff time.Duration) Option {
	return func(c *HTTPClient) {
		c.initialBackoff = backoff
	}
}

// WithBackoffMultiplier sets the multiplier factor for exponential backoff.
func WithBackoffMultiplier(multiplier float64) Option {
	return func(c *HTTPClient) {
		c.backoffMultiplier = multiplier
	}
}

// WithSleepFunc sets a custom sleep function for backoff delays (useful for fast unit testing).
func WithSleepFunc(fn func(time.Duration)) Option {
	return func(c *HTTPClient) {
		c.sleepFunc = fn
	}
}

// NewHTTPClient creates a new Firecrawl HTTP Client with standard defaults and optional parameters.
func NewHTTPClient(baseURL string, opts ...Option) *HTTPClient {
	c := &HTTPClient{
		baseURL:           baseURL,
		endpoint:          "/v1/extract",
		client:            &fasthttp.Client{},
		timeout:           30 * time.Second,
		maxRetries:        3,
		initialBackoff:    100 * time.Millisecond,
		backoffMultiplier: 2.0,
		sleepFunc:         nil,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// ExtractPDF streams the PDF from the given reader to Firecrawl and returns raw extraction output.
func (c *HTTPClient) ExtractPDF(ctx context.Context, r io.Reader) (*model.RawExtractionResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Prepare payload bytes (rewinding if io.Seeker, or reading once into memory)
	var bodyBytes []byte
	var err error

	if seeker, ok := r.(io.Seeker); ok {
		if _, seekErr := seeker.Seek(0, io.SeekStart); seekErr != nil {
			return nil, fmt.Errorf("failed to seek reader: %w", seekErr)
		}
	}

	bodyBytes, err = io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF stream: %w", err)
	}

	fullURL := strings.TrimRight(c.baseURL, "/") + "/" + strings.TrimLeft(c.endpoint, "/")

	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()

		req.SetRequestURI(fullURL)
		req.Header.SetMethod(fasthttp.MethodPost)
		req.Header.SetContentType("application/pdf")
		req.SetBody(bodyBytes)

		// Calculate timeout taking context deadline into account
		reqTimeout := c.timeout
		if deadline, ok := ctx.Deadline(); ok {
			timeUntil := time.Until(deadline)
			if timeUntil <= 0 {
				fasthttp.ReleaseRequest(req)
				fasthttp.ReleaseResponse(resp)
				return nil, ctx.Err()
			}
			if timeUntil < reqTimeout {
				reqTimeout = timeUntil
			}
		}

		doErr := c.client.DoTimeout(req, resp, reqTimeout)

		if doErr != nil {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)

			if err := ctx.Err(); err != nil {
				return nil, err
			}

			lastErr = doErr

			if attempt < c.maxRetries {
				if err := c.sleep(ctx, attempt); err != nil {
					return nil, err
				}
				continue
			}
			break
		}

		statusCode := resp.StatusCode()

		if statusCode == fasthttp.StatusOK {
			var result model.RawExtractionResult
			unmarshalErr := json.Unmarshal(resp.Body(), &result)

			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)

			if unmarshalErr != nil {
				return nil, fmt.Errorf("failed to decode json response: %w", unmarshalErr)
			}

			if result.Metadata == nil {
				result.Metadata = make(map[string]interface{})
			}
			if _, exists := result.Metadata["retry_count"]; !exists {
				result.Metadata["retry_count"] = attempt
			}

			return &result, nil
		}

		// 4xx errors fail fast (client-side errors, e.g. 400 Bad Request, 422 Unprocessable)
		if statusCode >= 400 && statusCode < 500 {
			errStr := fmt.Sprintf("client error (status %d): %s", statusCode, string(resp.Body()))
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)
			return nil, errors.New(errStr)
		}

		// 5xx errors are server-side failures (retryable)
		lastErr = fmt.Errorf("server error (status %d): %s", statusCode, string(resp.Body()))
		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(resp)

		if attempt < c.maxRetries {
			if err := c.sleep(ctx, attempt); err != nil {
				return nil, err
			}
			continue
		}
	}

	return nil, fmt.Errorf("%w: %v", ErrServiceUnavailable, lastErr)
}

func (c *HTTPClient) sleep(ctx context.Context, attempt int) error {
	backoff := time.Duration(float64(c.initialBackoff) * math.Pow(c.backoffMultiplier, float64(attempt)))

	if c.sleepFunc != nil {
		c.sleepFunc(backoff)
		return ctx.Err()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(backoff):
		return nil
	}
}
