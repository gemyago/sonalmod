package summarize

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	lp "github.com/gemyago/sonalmod/runtime/internal/llmproviders"
	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

const summarizerPromptSnippetMaxBytes = 200

// ProviderConfigsLister lists provider configuration used to pick the title LLM.
// Satisfied by [lp.ProvidersConfigService].
type ProviderConfigsLister interface {
	List(ctx context.Context) ([]lp.ProviderConfig, error)
}

// ModelResolver resolves a fully-qualified model name ("provider/model") to an ADK LLM.
// *ModelsLocator implements it in the runtime internal package; tests may substitute small fakes.
type ModelResolver interface {
	ResolveModel(ctx context.Context, fqModelName string) (model.LLM, error)
}

// LLMSummarizer uses a configured LLM for titles when available (see [NewLLMSummarizer]).
type LLMSummarizer struct {
	providers     ProviderConfigsLister
	modelResolver ModelResolver
	fallback      Summarizer
	promptSnippet *TruncatingSummarizer
	logger        *slog.Logger
}

// NewLLMSummarizer returns a summarizer that uses the first configured model
// with ModelConfig.Summarization set. Resolution is dynamic on each call. On failure or
// when no model is designated, it delegates to fallback (typically [NewTruncatingSummarizer] with no options).
func NewLLMSummarizer(
	providers ProviderConfigsLister,
	modelsLocator ModelResolver,
	fallback Summarizer,
	logger *slog.Logger,
) *LLMSummarizer {
	if logger == nil {
		logger = slog.Default()
	}
	return &LLMSummarizer{
		providers:     providers,
		modelResolver: modelsLocator,
		fallback:      fallback,
		promptSnippet: NewTruncatingSummarizer(WithTruncatingSummarizerMaxLen(summarizerPromptSnippetMaxBytes)),
		logger:        logger,
	}
}

func (s *LLMSummarizer) Summarize(ctx context.Context, text string) (string, error) {
	providers, err := s.providers.List(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "llm summarizer: list providers", "error", err)
		out, fbErr := s.fallback.Summarize(ctx, text)
		if fbErr != nil {
			return "", fmt.Errorf("fallback after list providers: %w", fbErr)
		}
		return out, nil
	}

	fq := firstSummarizationModelFQFromConfigs(providers)
	if fq == "" {
		return s.fallback.Summarize(ctx, text)
	}

	llm, err := s.modelResolver.ResolveModel(ctx, fq)
	if err != nil {
		s.logger.ErrorContext(ctx, "llm summarizer: resolve model",
			"model", fq, "error", err)
		out, fbErr := s.fallback.Summarize(ctx, text)
		if fbErr != nil {
			return "", fmt.Errorf("fallback after resolve model: %w", fbErr)
		}
		return out, nil
	}

	snippet, _ := s.promptSnippet.Summarize(ctx, text)
	prompt := "Generate a concise title (max 8 words, max 50 characters) for a conversation that starts with: " + snippet

	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{
				Role:  string(genai.RoleUser),
				Parts: []*genai.Part{{Text: prompt}},
			},
		},
	}

	title, err := collectLLMText(ctx, llm, req)
	if err != nil {
		s.logger.ErrorContext(ctx, "llm summarizer: llm generation", "model", fq, "error", err)
		out, fbErr := s.fallback.Summarize(ctx, text)
		if fbErr != nil {
			return "", fmt.Errorf("fallback after llm error: %w", fbErr)
		}
		return out, nil
	}
	title = strings.TrimSpace(title)
	if title == "" {
		s.logger.ErrorContext(ctx, "llm summarizer: empty llm output", "model", fq)
		out, fbErr := s.fallback.Summarize(ctx, text)
		if fbErr != nil {
			return "", fmt.Errorf("fallback after empty llm output: %w", fbErr)
		}
		return out, nil
	}

	return title, nil
}

func firstSummarizationModelFQFromConfigs(providers []lp.ProviderConfig) string {
	for _, p := range providers {
		for _, m := range p.Models {
			if m.Summarization {
				return p.Name + "/" + m.Name
			}
		}
	}
	return ""
}

func collectLLMText(ctx context.Context, llm model.LLM, req *model.LLMRequest) (string, error) {
	var b strings.Builder
	for resp, err := range llm.GenerateContent(ctx, req, false) {
		if err != nil {
			return "", err
		}
		if resp == nil || resp.Content == nil {
			continue
		}
		for _, part := range resp.Content.Parts {
			if part != nil && part.Text != "" {
				b.WriteString(part.Text)
			}
		}
	}
	return strings.TrimSpace(b.String()), nil
}
