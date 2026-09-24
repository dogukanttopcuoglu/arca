package verification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// debugWriter is the stderr sink for the per-call debug line; swapped in
// tests to capture structured output.
var debugWriter io.Writer = log.Default().Writer()

// HTTPJevChecker is the production EntailmentChecker seam (Phase 2): an HTTP
// client depending only on the TypeSafe System One contract
// (POST /v1/systemone with noul support/contradict questions), never on a
// server implementation. It carries the minimal runtime observability:
// atomic counters plus one structured debug line per call.
type HTTPJevChecker struct {
	url       string
	apiKey    string
	client    *http.Client
	threshold float64
	debug     bool

	requests     atomic.Int64
	failures     atomic.Int64
	latency      atomic.Int64
	lastDegraded atomic.Bool
}

// NewHTTPJevChecker constructs the adapter. timeout covers only the HTTP
// call; threshold is the support probability at or above which a relation
// is entailed. Every failure mode surfaces as an error so the pipeline
// degrades fail-open without touching the Phase 1 status.
func NewHTTPJevChecker(baseURL string, apiKey string, timeout time.Duration, threshold float64) *HTTPJevChecker {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if threshold <= 0 {
		threshold = 0.7
	}
	return &HTTPJevChecker{
		url:       strings.TrimRight(baseURL, "/"),
		apiKey:    apiKey,
		client:    &http.Client{Timeout: timeout},
		threshold: threshold,
		debug:     true,
	}
}

// CheckEntailment implements EntailmentChecker: asks Jev for support and
// contradict probabilities on the (claim, source) pair and maps them to a
// relation. Score is the support probability in all cases.
func (c *HTTPJevChecker) CheckEntailment(ctx context.Context, claim, sourceText string) (EntailmentScore, error) {
	start := time.Now()
	c.requests.Add(1)
	c.lastDegraded.Store(false)

	support, contradict, relation, err := c.call(ctx, claim, sourceText)

	latency := time.Since(start).Milliseconds()
	c.latency.Add(latency)
	if err != nil {
		c.failures.Add(1)
		c.lastDegraded.Store(true)
	}
	if c.debug {
		errField := ""
		if err != nil {
			errField = " error=" + strconv.Quote(err.Error())
		}
		fmt.Fprintf(debugWriter, "jev_verify: claim_len=%d source_len=%d latency_ms=%d support=%.3f contradict=%.3f relation=%s degraded=%v%s\n",
			len(claim), len(sourceText), latency, support, contradict, relation, c.lastDegraded.Load(), errField)
	}
	if err != nil {
		return EntailmentScore{}, err
	}
	return EntailmentScore{Score: support, Relation: relation}, nil
}

// RequestsTotal reports the total entailment checks (atomic counter).
func (c *HTTPJevChecker) RequestsTotal() int64 { return c.requests.Load() }

// FailuresTotal reports the total failed entailment checks (atomic counter).
func (c *HTTPJevChecker) FailuresTotal() int64 { return c.failures.Load() }

// LatencyMsTotal reports the summed entailment latency in milliseconds (atomic).
func (c *HTTPJevChecker) LatencyMsTotal() int64 { return c.latency.Load() }

// LastDegraded reports whether the last call failed.
func (c *HTTPJevChecker) LastDegraded() bool { return c.lastDegraded.Load() }

const (
	supportInstructions    = "Does the source text support the claim? Answer based only on the source text, not on general knowledge. Claim: `state.claim`, Source: `state.source`"
	contradictInstructions = "Does the source text contradict the claim? Answer based only on the source text, not on general knowledge. Claim: `state.claim`, Source: `state.source`"
	maxSourceRunes         = 2000
)

type jevRequest struct {
	Model     string       `json:"model"`
	State     jevState     `json:"state"`
	Questions jevQuestions `json:"questions"`
}

type jevState struct {
	Claim  string `json:"claim"`
	Source string `json:"source"`
}

type jevQuestions struct {
	Support    jevQuestion `json:"support"`
	Contradict jevQuestion `json:"contradict"`
}

type jevQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
}

type jevResponse struct {
	Answers struct {
		Support struct {
			Noul *float64 `json:"noul"`
		} `json:"support"`
		Contradict struct {
			Noul *float64 `json:"noul"`
		} `json:"contradict"`
	} `json:"answers"`
}

// call performs one HTTP round trip, validates the contract response, and
// maps the noul probabilities to a relation.
func (c *HTTPJevChecker) call(ctx context.Context, claim, sourceText string) (float64, float64, string, error) {
	reqBody := jevRequest{
		Model: "jev-latest",
		State: jevState{Claim: claim, Source: clipRunes(sourceText, maxSourceRunes)},
		Questions: jevQuestions{
			Support:    jevQuestion{Type: "noul", Instructions: supportInstructions},
			Contradict: jevQuestion{Type: "noul", Instructions: contradictInstructions},
		},
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return 0, 0, "", fmt.Errorf("jev request marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/v1/systemone", bytes.NewReader(raw))
	if err != nil {
		return 0, 0, "", fmt.Errorf("jev request build: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, 0, "", fmt.Errorf("jev call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
	if err != nil {
		return 0, 0, "", fmt.Errorf("jev response read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, 0, "", fmt.Errorf("jev status %d", resp.StatusCode)
	}

	var parsed jevResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, 0, "", fmt.Errorf("jev invalid response: %w", err)
	}
	if parsed.Answers.Support.Noul == nil || parsed.Answers.Contradict.Noul == nil {
		return 0, 0, "", fmt.Errorf("jev invalid response: missing support/contradict scores")
	}

	support := *parsed.Answers.Support.Noul
	contradict := *parsed.Answers.Contradict.Noul
	relation := "neutral"
	switch {
	case support >= c.threshold:
		relation = "entailed"
	case contradict >= c.threshold:
		relation = "contradicted"
	}
	return support, contradict, relation, nil
}

func clipRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
