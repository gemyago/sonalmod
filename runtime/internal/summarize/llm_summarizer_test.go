package summarize

import (
	"context"
	"errors"
	"iter"
	"log/slog"
	"testing"

	lp "github.com/gemyago/sonalmod/runtime/internal/llmproviders"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.DiscardHandler).With("test", t.Name())
}

func TestLLMSummarizer(t *testing.T) {
	t.Parallel()

	fake := faker.New()

	newLister := func(t *testing.T, fn func(context.Context) ([]lp.ProviderConfig, error)) *fakeProvidersLister {
		t.Helper()
		return &fakeProvidersLister{fn: fn}
	}

	t.Run("no summarization model designated falls back to truncation", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		log := testLogger(t)
		provName := fake.Lorem().Word()
		modelName := fake.Lorem().Word()
		lister := newLister(t, func(context.Context) ([]lp.ProviderConfig, error) {
			return []lp.ProviderConfig{
				{
					Name: provName,
					Models: []lp.ModelConfig{
						{Name: modelName, Summarization: false},
					},
				},
			}, nil
		})

		long := "word " + fake.Lorem().Sentence(40)
		fallback := NewTruncatingSummarizer()
		want, err := fallback.Summarize(ctx, long)
		require.NoError(t, err)

		sum := NewLLMSummarizer(lister, &stubModelResolver{}, fallback, log)
		got, err := sum.Summarize(ctx, long)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("summarization model designated calls LLM and returns title", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		log := testLogger(t)
		provName := fake.Lorem().Word()
		modelName := fake.Lorem().Word()
		fq := provName + "/" + modelName
		title := fake.Lorem().Sentence(4)
		lister := newLister(t, func(context.Context) ([]lp.ProviderConfig, error) {
			return []lp.ProviderConfig{
				{
					Name: provName,
					Models: []lp.ModelConfig{
						{Name: modelName, Summarization: true},
					},
				},
			}, nil
		})

		resolver := &stubModelResolver{
			fn: func(_ context.Context, gotFQ string) (model.LLM, error) {
				require.Equal(t, fq, gotFQ)
				return fixedTextLLM{text: title}, nil
			},
		}

		sum := NewLLMSummarizer(lister, resolver, NewTruncatingSummarizer(), log)
		got, err := sum.Summarize(ctx, fake.Lorem().Paragraph(2))
		require.NoError(t, err)
		require.Equal(t, title, got)
	})

	t.Run("LLM error falls back to truncation", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		log := testLogger(t)
		provName := fake.Lorem().Word()
		modelName := fake.Lorem().Word()
		lister := newLister(t, func(context.Context) ([]lp.ProviderConfig, error) {
			return []lp.ProviderConfig{
				{
					Name: provName,
					Models: []lp.ModelConfig{
						{Name: modelName, Summarization: true},
					},
				},
			}, nil
		})

		resolver := &stubModelResolver{
			fn: func(context.Context, string) (model.LLM, error) {
				return errLLM{}, nil
			},
		}

		long := "truncateme " + fake.Lorem().Sentence(30)
		fallback := NewTruncatingSummarizer()
		want, err := fallback.Summarize(ctx, long)
		require.NoError(t, err)

		sum := NewLLMSummarizer(lister, resolver, fallback, log)
		got, err := sum.Summarize(ctx, long)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("provider list error falls back to truncation", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		log := testLogger(t)
		lister := newLister(t, func(context.Context) ([]lp.ProviderConfig, error) {
			return nil, errors.New("list boom")
		})

		long := "x " + fake.Lorem().Sentence(25)
		fallback := NewTruncatingSummarizer()
		want, err := fallback.Summarize(ctx, long)
		require.NoError(t, err)

		sum := NewLLMSummarizer(lister, &stubModelResolver{}, fallback, log)
		got, err := sum.Summarize(ctx, long)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("nil logger uses slog default without panic", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		lister := newLister(t, func(context.Context) ([]lp.ProviderConfig, error) {
			return []lp.ProviderConfig{}, nil
		})

		sum := NewLLMSummarizer(lister, &stubModelResolver{}, NewTruncatingSummarizer(), nil)
		_, err := sum.Summarize(ctx, "hi")
		require.NoError(t, err)
	})

	t.Run("resolve model error falls back to truncation", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		log := testLogger(t)
		provName := fake.Lorem().Word()
		modelName := fake.Lorem().Word()
		lister := newLister(t, func(context.Context) ([]lp.ProviderConfig, error) {
			return []lp.ProviderConfig{
				{
					Name: provName,
					Models: []lp.ModelConfig{
						{Name: modelName, Summarization: true},
					},
				},
			}, nil
		})

		resolver := &stubModelResolver{
			fn: func(context.Context, string) (model.LLM, error) {
				return nil, errors.New("resolve failed")
			},
		}

		long := "y " + fake.Lorem().Sentence(28)
		fallback := NewTruncatingSummarizer()
		want, err := fallback.Summarize(ctx, long)
		require.NoError(t, err)

		sum := NewLLMSummarizer(lister, resolver, fallback, log)
		got, err := sum.Summarize(ctx, long)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("empty LLM text falls back to truncation", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		log := testLogger(t)
		provName := fake.Lorem().Word()
		modelName := fake.Lorem().Word()
		lister := newLister(t, func(context.Context) ([]lp.ProviderConfig, error) {
			return []lp.ProviderConfig{
				{
					Name: provName,
					Models: []lp.ModelConfig{
						{Name: modelName, Summarization: true},
					},
				},
			}, nil
		})

		resolver := &stubModelResolver{
			fn: func(context.Context, string) (model.LLM, error) {
				return fixedTextLLM{text: "   "}, nil
			},
		}

		long := "z " + fake.Lorem().Sentence(22)
		fallback := NewTruncatingSummarizer()
		want, err := fallback.Summarize(ctx, long)
		require.NoError(t, err)

		sum := NewLLMSummarizer(lister, resolver, fallback, log)
		got, err := sum.Summarize(ctx, long)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("list providers error with failing fallback returns wrapped error", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		log := testLogger(t)
		lister := newLister(t, func(context.Context) ([]lp.ProviderConfig, error) {
			return nil, errors.New("list failed")
		})

		sum := NewLLMSummarizer(lister, &stubModelResolver{}, errFallbackSummarizer{}, log)
		_, err := sum.Summarize(ctx, "x")
		require.Error(t, err)
		require.ErrorContains(t, err, "fallback after list providers")
	})

	t.Run("resolve error with failing fallback returns wrapped error", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		log := testLogger(t)
		p := fake.Lorem().Word()
		m := fake.Lorem().Word()
		lister := newLister(t, func(context.Context) ([]lp.ProviderConfig, error) {
			return []lp.ProviderConfig{
				{Name: p, Models: []lp.ModelConfig{{Name: m, Summarization: true}}},
			}, nil
		})

		resolver := &stubModelResolver{
			fn: func(context.Context, string) (model.LLM, error) {
				return nil, errors.New("no model")
			},
		}

		sum := NewLLMSummarizer(lister, resolver, errFallbackSummarizer{}, log)
		_, err := sum.Summarize(ctx, "x")
		require.Error(t, err)
		require.ErrorContains(t, err, "fallback after resolve model")
	})

	t.Run("LLM generation error with failing fallback returns wrapped error", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		log := testLogger(t)
		p := fake.Lorem().Word()
		m := fake.Lorem().Word()
		lister := newLister(t, func(context.Context) ([]lp.ProviderConfig, error) {
			return []lp.ProviderConfig{
				{Name: p, Models: []lp.ModelConfig{{Name: m, Summarization: true}}},
			}, nil
		})

		resolver := &stubModelResolver{
			fn: func(context.Context, string) (model.LLM, error) {
				return errLLM{}, nil
			},
		}

		sum := NewLLMSummarizer(lister, resolver, errFallbackSummarizer{}, log)
		_, err := sum.Summarize(ctx, "x")
		require.Error(t, err)
		require.ErrorContains(t, err, "fallback after llm error")
	})

	t.Run("empty LLM with failing fallback returns wrapped error", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		log := testLogger(t)
		p := fake.Lorem().Word()
		m := fake.Lorem().Word()
		lister := newLister(t, func(context.Context) ([]lp.ProviderConfig, error) {
			return []lp.ProviderConfig{
				{Name: p, Models: []lp.ModelConfig{{Name: m, Summarization: true}}},
			}, nil
		})

		resolver := &stubModelResolver{
			fn: func(context.Context, string) (model.LLM, error) {
				return fixedTextLLM{text: ""}, nil
			},
		}

		sum := NewLLMSummarizer(lister, resolver, errFallbackSummarizer{}, log)
		_, err := sum.Summarize(ctx, "x")
		require.Error(t, err)
		require.ErrorContains(t, err, "fallback after empty llm output")
	})

	t.Run("collectLLMText skips nil content then concatenates parts", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		a := fake.Lorem().Word()
		b := fake.Lorem().Word()
		llm := seqLLM{
			yields: []*model.LLMResponse{
				{Content: nil},
				{
					Content: &genai.Content{
						Parts: []*genai.Part{nil, {Text: a}, {Text: b}},
					},
				},
			},
		}
		got, err := collectLLMText(ctx, llm, &model.LLMRequest{})
		require.NoError(t, err)
		require.Equal(t, a+b, got)
	})

	t.Run("multiple models with summarization uses first found", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		log := testLogger(t)
		p1 := fake.Lorem().Word()
		p2 := fake.Lorem().Word()
		m1 := fake.Lorem().Word()
		m2 := fake.Lorem().Word()
		fqFirst := p1 + "/" + m1
		title := fake.Lorem().Word()

		lister := newLister(t, func(context.Context) ([]lp.ProviderConfig, error) {
			return []lp.ProviderConfig{
				{
					Name: p1,
					Models: []lp.ModelConfig{
						{Name: m1, Summarization: true},
					},
				},
				{
					Name: p2,
					Models: []lp.ModelConfig{
						{Name: m2, Summarization: true},
					},
				},
			}, nil
		})

		var sawFQ string
		resolver := &stubModelResolver{
			fn: func(_ context.Context, fq string) (model.LLM, error) {
				sawFQ = fq
				require.Equal(t, fqFirst, fq)
				return fixedTextLLM{text: title}, nil
			},
		}

		sum := NewLLMSummarizer(lister, resolver, NewTruncatingSummarizer(), log)
		got, err := sum.Summarize(ctx, fake.Lorem().Sentence(8))
		require.NoError(t, err)
		require.Equal(t, title, got)
		require.Equal(t, fqFirst, sawFQ)
	})
}

type fakeProvidersLister struct {
	fn func(context.Context) ([]lp.ProviderConfig, error)
}

func (f *fakeProvidersLister) List(ctx context.Context) ([]lp.ProviderConfig, error) {
	if f.fn != nil {
		return f.fn(ctx)
	}
	return nil, nil
}

type stubModelResolver struct {
	fn func(ctx context.Context, fqModelName string) (model.LLM, error)
}

func (s *stubModelResolver) ResolveModel(ctx context.Context, fqModelName string) (model.LLM, error) {
	if s.fn != nil {
		return s.fn(ctx, fqModelName)
	}
	return nil, errors.New("stubModelResolver: not configured")
}

type fixedTextLLM struct {
	text string
}

func (f fixedTextLLM) Name() string { return "fixed" }

func (f fixedTextLLM) GenerateContent(
	_ context.Context,
	_ *model.LLMRequest,
	_ bool,
) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{
			Content: &genai.Content{
				Parts: []*genai.Part{{Text: f.text}},
			},
		}, nil)
	}
}

type errLLM struct{}

func (errLLM) Name() string { return "err" }

func (errLLM) GenerateContent(
	_ context.Context,
	_ *model.LLMRequest,
	_ bool,
) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(nil, errors.New("llm failed"))
	}
}

type errFallbackSummarizer struct{}

func (errFallbackSummarizer) Summarize(context.Context, string) (string, error) {
	return "", errors.New("fallback failed")
}

type seqLLM struct {
	yields []*model.LLMResponse
}

func (s seqLLM) Name() string { return "seq" }

func (s seqLLM) GenerateContent(
	_ context.Context,
	_ *model.LLMRequest,
	_ bool,
) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		for _, r := range s.yields {
			if !yield(r, nil) {
				return
			}
		}
	}
}
