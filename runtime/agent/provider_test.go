package agent

import (
	"net/http"
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/require"
)

func TestOpenAICompatibleLLMProvider(t *testing.T) {
	fake := faker.New()
	p := NewOpenAICompatibleLLMProvider(OpenAICompatibleLLMProviderArgs{
		Name:    fake.Lorem().Word(),
		APIKey:  fake.Lorem().Word(),
		BaseURL: fake.Internet().URL(),
	}, OpenAIWithHTTPClient(http.DefaultClient))
	require.NotNil(t, p)
}
