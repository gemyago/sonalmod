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

func TestClient_Crawl(t *testing.T) {
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
		// Arrange - Use randomized data
		requestURL := "https://" + fake.Internet().Domain()
		prompt := fake.Lorem().Sentence(5)
		excludePath := fake.Lorem().Word()
		includePath := fake.Lorem().Word()
		maxDiscoveryDepth := 2 + fake.RandomNumber(5)
		limit := 100 + fake.RandomNumber(1000)

		// Prepare expected response with randomized data
		responseID := "crawl-" + fake.UUID().V4()
		responseURL := "https://" + fake.Internet().Domain() + "/" + fake.Lorem().Word()

		expectedResponse := &CrawlResponse{
			Success: true,
			ID:      responseID,
			URL:     responseURL,
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify request details
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "/crawl", r.URL.Path)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			// Return complete successful response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			fmt.Fprintf(w, `{
				"success": %t,
				"id": "%s",
				"url": "%s"
			}`, expectedResponse.Success, expectedResponse.ID, expectedResponse.URL)
		}))
		defer server.Close()

		deps, opts := makeClientParams(t, server.URL)
		client := NewClient(deps, opts...)

		req := &CrawlRequest{
			URL:                   requestURL,
			Prompt:                prompt,
			ExcludePaths:          []string{excludePath},
			IncludePaths:          []string{includePath},
			MaxDiscoveryDepth:     maxDiscoveryDepth,
			Sitemap:               "include",
			IgnoreQueryParameters: true,
			RegexOnFullURL:        true,
			Limit:                 limit,
			CrawlEntireDomain:     true,
			AllowExternalLinks:    false,
			AllowSubdomains:       true,
			Delay:                 0.5,
			MaxConcurrency:        5,
			Webhook: &Webhook{
				URL:    "https://" + fake.Internet().Domain() + "/webhook",
				Events: []string{"completed", "failed"},
			},
			ScrapeOptions: &ScrapeOptions{
				OnlyMainContent: true,
				Timeout:         30000,
			},
			ZeroDataRetention: false,
		}

		// Act
		result, err := client.Crawl(t.Context(), CrawlParams{
			Request: req,
		})

		// Assert
		require.NoError(t, err)
		assert.Equal(t, expectedResponse, result)
	})

	t.Run("success with required parameters only", func(t *testing.T) {
		// Arrange - Use randomized data
		requestURL := "https://" + fake.Internet().Domain()

		// Prepare expected minimal response with randomized data
		expectedID := "crawl-" + fake.UUID().V4()
		expectedURL := "https://" + fake.Internet().Domain()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			// Return minimal successful response with randomized data
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{
				"success": true,
				"id": "%s",
				"url": "%s"
			}`, expectedID, expectedURL)
		}))
		defer server.Close()

		deps, opts := makeClientParams(t, server.URL)
		client := NewClient(deps, opts...)

		req := &CrawlRequest{
			URL: requestURL,
		}

		// Act
		result, err := client.Crawl(t.Context(), CrawlParams{
			Request: req,
		})

		// Assert
		require.NoError(t, err)
		assert.Equal(t, expectedID, result.ID)
		assert.Equal(t, expectedURL, result.URL)
		assert.True(t, result.Success)
	})

	t.Run("handles API error", func(t *testing.T) {
		// Arrange
		requestURL := "https://" + fake.Internet().Domain()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error": "Invalid request"}`)
		}))
		defer server.Close()

		deps, opts := makeClientParams(t, server.URL)
		client := NewClient(deps, opts...)

		req := &CrawlRequest{
			URL: requestURL,
		}

		// Act
		result, err := client.Crawl(t.Context(), CrawlParams{
			Request: req,
		})

		// Assert
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "failed to start crawl")
	})
}
