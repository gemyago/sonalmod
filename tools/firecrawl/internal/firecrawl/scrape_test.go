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

func TestClient_Scrape(t *testing.T) {
	fake := faker.New()

	makeClientParams := func(_ *testing.T, baseURL string) (ClientArgs, []ClientOption) {
		logger := slog.New(slog.DiscardHandler)
		return ClientArgs{
				BaseURL:   baseURL,
				AuthToken: fake.UUID().V4(),
			}, []ClientOption{
				WithLogger(logger),
			}
	}

	t.Run("success with all parameters and fields", func(t *testing.T) {
		// Arrange - random data
		requestURL := "https://" + fake.Internet().Domain()
		// fill some options
		expectedMarkdown := fake.Lorem().Paragraph(1)
		expectedSummary := fake.Lorem().Sentence(3)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "/scrape", r.URL.Path)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{
                "success": true,
                "data": {
                    "markdown": "%s",
                    "summary": "%s"
                }
            }`, expectedMarkdown, expectedSummary)
		}))
		defer server.Close()

		deps, opts := makeClientParams(t, server.URL)
		client := NewClient(deps, opts...)

		req := &ScrapeRequest{
			URL: requestURL,
			ScrapeOptions: ScrapeOptions{
				OnlyMainContent: true,
				Mobile:          true,
				Timeout:         10000,
			},
			ZeroDataRetention: true,
		}

		// Act
		result, err := client.Scrape(t.Context(), ScrapeParams{Request: req})

		// Assert
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Success)
		require.NotNil(t, result.Data)
		assert.Equal(t, expectedMarkdown, result.Data.Markdown)
		assert.Equal(t, expectedSummary, result.Data.Summary)
	})

	t.Run("success with required parameters only", func(t *testing.T) {
		requestURL := "https://" + fake.Internet().Domain()
		expectedMarkdown := fake.Lorem().Sentence(2)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{
                "success": true,
                "data": { "markdown": "%s" }
            }`, expectedMarkdown)
		}))
		defer server.Close()

		deps, opts := makeClientParams(t, server.URL)
		client := NewClient(deps, opts...)

		req := &ScrapeRequest{
			URL: requestURL,
		}

		result, err := client.Scrape(t.Context(), ScrapeParams{Request: req})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Success)
		require.NotNil(t, result.Data)
		assert.Equal(t, expectedMarkdown, result.Data.Markdown)
	})

	t.Run("handles API error", func(t *testing.T) {
		requestURL := "https://" + fake.Internet().Domain()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{ "error": "oops" }`)
		}))
		defer server.Close()

		deps, opts := makeClientParams(t, server.URL)
		client := NewClient(deps, opts...)

		req := &ScrapeRequest{
			URL: requestURL,
		}

		result, err := client.Scrape(t.Context(), ScrapeParams{Request: req})

		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "failed to scrape url")
	})
}
