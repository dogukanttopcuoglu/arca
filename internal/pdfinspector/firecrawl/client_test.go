package firecrawl_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"arca/internal/pdfinspector/firecrawl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractPDF_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/extract", r.URL.Path)
		assert.Equal(t, "application/pdf", r.Header.Get("Content-Type"))

		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, []byte("fake-pdf-data"), body)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"markdown": "# Sample PDF\n\nContent here.",
			"json_layout": {"pages": 1},
			"metadata": {"title": "Sample"},
			"ocr_applied": true
		}`))
	}))
	defer ts.Close()

	client := firecrawl.NewHTTPClient(
		ts.URL,
		firecrawl.WithEndpoint("/v1/extract"),
	)

	pdfStream := bytes.NewReader([]byte("fake-pdf-data"))
	res, err := client.ExtractPDF(context.Background(), pdfStream)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "# Sample PDF\n\nContent here.", res.Markdown)
	assert.True(t, res.OCRApplied)
	assert.Equal(t, "Sample", res.Metadata["title"])
}

func TestExtractPDF_RetrySuccess(t *testing.T) {
	var attempts int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"markdown": "# Recovered Document",
			"json_layout": {},
			"metadata": {},
			"ocr_applied": false
		}`))
	}))
	defer ts.Close()

	sleepCalls := 0
	client := firecrawl.NewHTTPClient(
		ts.URL,
		firecrawl.WithMaxRetries(3),
		firecrawl.WithInitialBackoff(10*time.Millisecond),
		firecrawl.WithBackoffMultiplier(2.0),
		firecrawl.WithSleepFunc(func(d time.Duration) {
			sleepCalls++
		}),
	)

	pdfStream := bytes.NewReader([]byte("pdf-payload-for-retry"))
	res, err := client.ExtractPDF(context.Background(), pdfStream)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "# Recovered Document", res.Markdown)
	assert.Equal(t, int32(3), atomic.LoadInt32(&attempts))
	assert.Equal(t, 2, sleepCalls)
}

func TestExtractPDF_ExhaustedRetries(t *testing.T) {
	var attempts int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	client := firecrawl.NewHTTPClient(
		ts.URL,
		firecrawl.WithMaxRetries(2),
		firecrawl.WithSleepFunc(func(d time.Duration) {}),
	)

	pdfStream := bytes.NewBufferString("pdf-data")
	res, err := client.ExtractPDF(context.Background(), pdfStream)

	require.Error(t, err)
	require.Nil(t, res)
	assert.True(t, errors.Is(err, firecrawl.ErrServiceUnavailable))
	assert.Equal(t, int32(3), atomic.LoadInt32(&attempts)) // 1 initial + 2 retries
}

func TestExtractPDF_ContextCanceled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := firecrawl.NewHTTPClient(ts.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	pdfStream := bytes.NewReader([]byte("pdf-data"))
	res, err := client.ExtractPDF(ctx, pdfStream)

	require.Error(t, err)
	require.Nil(t, res)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestExtractPDF_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := firecrawl.NewHTTPClient(
		ts.URL,
		firecrawl.WithTimeout(20*time.Millisecond),
		firecrawl.WithMaxRetries(0),
	)

	pdfStream := bytes.NewReader([]byte("pdf-data"))
	res, err := client.ExtractPDF(context.Background(), pdfStream)

	require.Error(t, err)
	require.Nil(t, res)
	assert.True(t, errors.Is(err, firecrawl.ErrServiceUnavailable))
}

func TestExtractPDF_ClientError(t *testing.T) {
	var attempts int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "Invalid PDF format"}`))
	}))
	defer ts.Close()

	client := firecrawl.NewHTTPClient(
		ts.URL,
		firecrawl.WithMaxRetries(3),
	)

	pdfStream := bytes.NewReader([]byte("bad-pdf-data"))
	res, err := client.ExtractPDF(context.Background(), pdfStream)

	require.Error(t, err)
	require.Nil(t, res)
	assert.False(t, errors.Is(err, firecrawl.ErrServiceUnavailable))
	assert.Equal(t, int32(1), atomic.LoadInt32(&attempts)) // No retries on 400 Bad Request
}

func TestExtractPDF_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-a-valid-json`))
	}))
	defer ts.Close()

	client := firecrawl.NewHTTPClient(
		ts.URL,
		firecrawl.WithMaxRetries(0),
	)

	pdfStream := bytes.NewReader([]byte("pdf-data"))
	res, err := client.ExtractPDF(context.Background(), pdfStream)

	require.Error(t, err)
	require.Nil(t, res)
}
