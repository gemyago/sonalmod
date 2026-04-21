package firecrawl

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripRecorder struct {
	n  int
	rt http.RoundTripper
}

func (r *roundTripRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	r.n++
	if r.rt == nil {
		r.rt = http.DefaultTransport
	}
	return r.rt.RoundTrip(req)
}

func TestNewClient_WithHTTPClient_usesInjectedClient(t *testing.T) {
	fake := faker.New()
	requestURL := "https://" + fake.Internet().Domain()
	expectedMarkdown := fake.Lorem().Sentence(2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"success": true, "data": { "markdown": "%s" }}`, expectedMarkdown)
	}))
	t.Cleanup(server.Close)

	rec := &roundTripRecorder{}
	injected := &http.Client{Transport: rec}

	client := NewClient(ClientArgs{BaseURL: server.URL},
		WithLogger(slog.New(slog.DiscardHandler)),
		WithHTTPClient(injected),
	)

	result, err := client.Scrape(t.Context(), ScrapeParams{
		Request: &ScrapeRequest{URL: requestURL},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, rec.n, "injected client transport should have been used")
	assert.True(t, result.Success)
	require.NotNil(t, result.Data)
	assert.Equal(t, expectedMarkdown, result.Data.Markdown)
}
