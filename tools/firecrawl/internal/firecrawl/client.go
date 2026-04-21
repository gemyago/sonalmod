package firecrawl

import (
	"log/slog"
	"net/http"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
	logger     *slog.Logger
}

type ClientArgs struct {
	BaseURL   string
	AuthToken string `json:"-"`
}

type clientOpts struct {
	httpClient *http.Client
	logger     *slog.Logger
}

type ClientOption func(*clientOpts)

func WithLogger(logger *slog.Logger) ClientOption {
	return func(c *clientOpts) {
		c.logger = logger
	}
}

// WithHTTPClient sets the HTTP client used for API requests. A nil client is ignored.
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *clientOpts) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

func NewClient(
	deps ClientArgs,
	opts ...ClientOption,
) *Client {
	cOpts := clientOpts{
		httpClient: http.DefaultClient,
		logger:     slog.Default(),
	}

	for _, opt := range opts {
		opt(&cOpts)
	}

	return &Client{
		httpClient: cOpts.httpClient,
		baseURL:    deps.BaseURL,
		logger:     cOpts.logger,
	}
}
