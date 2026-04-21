package agent

import (
	"net/http"

	genkitApi "github.com/firebase/genkit/go/core/api"
	oai "github.com/firebase/genkit/go/plugins/compat_oai"
	"github.com/openai/openai-go/option"
)

// Provider allows defining various LLM providers.
type Provider struct {
	genkitPlugin genkitApi.Plugin
}

type OpenAICompatibleLLMProviderArgs struct {
	// Name is the name of the provider.
	// This will be used as a prefix for model names (e.g., "myprovider/model-name").
	Name string

	// APIKey is the API key to use with the provider.
	APIKey string `json:"-"`

	// BaseURL is the base URL to use with the provider.
	BaseURL string
}

type OpenAICompatibleLLMProviderOpt func(*oai.OpenAICompatible)

// OpenAIWithHTTPClient sets the HTTP client to use with the provider.
func OpenAIWithHTTPClient(client *http.Client) OpenAICompatibleLLMProviderOpt {
	return func(plugin *oai.OpenAICompatible) {
		plugin.Opts = append(plugin.Opts, option.WithHTTPClient(client))
	}
}

func NewOpenAICompatibleLLMProvider(
	args OpenAICompatibleLLMProviderArgs,
	opts ...OpenAICompatibleLLMProviderOpt,
) *Provider {
	plugin := &oai.OpenAICompatible{
		Provider: args.Name,
		APIKey:   args.APIKey,
		BaseURL:  args.BaseURL,
	}
	for _, opt := range opts {
		opt(plugin)
	}
	return &Provider{
		genkitPlugin: plugin,
	}
}
