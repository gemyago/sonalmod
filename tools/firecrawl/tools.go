package firecrawl

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gemyago/sonalmod/runtime/agent"
	ifc "github.com/gemyago/sonalmod/tools/firecrawl/internal/firecrawl"
)

type registerToolsOpts struct {
	baseURL    string
	authToken  string `json:"-"`
	logger     *slog.Logger
	httpClient *http.Client
}

type RegisterToolsOpt func(*registerToolsOpts)

func WithBaseURL(baseURL string) RegisterToolsOpt {
	return func(opts *registerToolsOpts) {
		opts.baseURL = baseURL
	}
}

func WithAuthToken(authToken string) RegisterToolsOpt {
	return func(opts *registerToolsOpts) {
		opts.authToken = authToken
	}
}

// WithClientLogger sets the slog logger for the Firecrawl HTTP client. Nil is ignored.
func WithClientLogger(logger *slog.Logger) RegisterToolsOpt {
	return func(opts *registerToolsOpts) {
		opts.logger = logger
	}
}

// WithClientHTTPClient sets the HTTP client used for Firecrawl API requests. Nil is ignored.
func WithClientHTTPClient(httpClient *http.Client) RegisterToolsOpt {
	return func(opts *registerToolsOpts) {
		opts.httpClient = httpClient
	}
}

type ToolsRegistry interface {
	AddTools(tools ...agent.DefinedTool)
}

func RegisterTools(registry ToolsRegistry, opts ...RegisterToolsOpt) {
	o := registerToolsOpts{}
	for _, opt := range opts {
		opt(&o)
	}
	if strings.TrimSpace(o.baseURL) == "" {
		return
	}

	var clientOpts []ifc.ClientOption
	if o.logger != nil {
		clientOpts = append(clientOpts, ifc.WithLogger(o.logger))
	}
	if o.httpClient != nil {
		clientOpts = append(clientOpts, ifc.WithHTTPClient(o.httpClient))
	}

	client := ifc.NewClient(ifc.ClientArgs{
		BaseURL:   o.baseURL,
		AuthToken: o.authToken,
	}, clientOpts...)

	registry.AddTools(firecrawlAgentTools(client)...)
}
