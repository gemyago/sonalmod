package agent

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/gemyago/sonalmod/runtime/internal"
	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	lp "github.com/gemyago/sonalmod/runtime/internal/llmproviders"
	"github.com/gemyago/sonalmod/runtime/internal/sessions"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/agent/llmagent"
	adkModel "google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

func TestRunner(t *testing.T) {
	fake := faker.New()
	rootTestLogger := internal.RootTestLogger()
	t.Run("NewRunner", func(t *testing.T) {
		t.Run("succeeds with ProvidersConfigService", func(t *testing.T) {
			providersSvc := lp.NewMockProvidersConfigService(t)
			runner, err := NewRunner(RunnerArgs{
				ProvidersConfigService: providersSvc,
			}, WithLogger(rootTestLogger))
			require.NoError(t, err)
			require.NotNil(t, runner)
		})

		t.Run("succeeds with optional tools registry", func(t *testing.T) {
			runner, err := NewRunner(RunnerArgs{
				ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			},
				WithLogger(rootTestLogger),
				WithToolsRegistry(NewToolsRegistry()),
			)
			require.NoError(t, err)
			require.NotNil(t, runner)
		})

		t.Run("succeeds with system prompt fragments", func(t *testing.T) {
			runner, err := NewRunner(RunnerArgs{
				ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			},
				WithLogger(rootTestLogger),
				WithSystemPromptFragments(SystemPromptFragment{
					Section: fake.Lorem().Word(),
					Content: fake.Lorem().Sentence(5),
				}),
			)
			require.NoError(t, err)
			require.NotNil(t, runner)
		})

		t.Run("returns error when ProvidersConfigService is nil", func(t *testing.T) {
			_, err := NewRunner(RunnerArgs{})
			require.Error(t, err)
			require.ErrorContains(t, err, "providers config service is required")
		})

		t.Run("succeeds with file system storage", func(t *testing.T) {
			dir := t.TempDir()
			runner, err := NewRunner(RunnerArgs{
				ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			}, WithLogger(rootTestLogger), WithFileSystemStorage(dir))
			require.NoError(t, err)
			require.NotNil(t, runner)
		})

		t.Run("returns error when file storage base dir is invalid", func(t *testing.T) {
			_, err := NewRunner(RunnerArgs{
				ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			}, WithLogger(rootTestLogger), WithFileSystemStorage(""))
			require.Error(t, err)
			require.ErrorContains(t, err, "session storage")
		})

		t.Run("succeeds with database storage", func(t *testing.T) {
			runner, err := NewRunner(RunnerArgs{
				ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			}, WithLogger(rootTestLogger), WithDatabaseStorage(":memory:"))
			require.NoError(t, err)
			require.NotNil(t, runner)
		})

		t.Run("succeeds with database storage and custom table prefix", func(t *testing.T) {
			prefix := fake.Lexify("?????_")
			runner, err := NewRunner(RunnerArgs{
				ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			},
				WithLogger(rootTestLogger),
				WithDatabaseStorage(":memory:"),
				WithDatabaseTablePrefix(prefix),
			)
			require.NoError(t, err)
			require.NotNil(t, runner)
		})

		t.Run("returns error when database DSN is invalid", func(t *testing.T) {
			_, err := NewRunner(RunnerArgs{
				ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			}, WithLogger(rootTestLogger), WithDatabaseStorage("host=bad port=0 dbname=none"))
			require.Error(t, err)
			require.ErrorContains(t, err, "session storage")
		})

		t.Run("WithDatabaseStorage clears file storage flag", func(t *testing.T) {
			opts := &runnerOpts{}
			WithFileSystemStorage(t.TempDir())(opts)
			WithDatabaseStorage(":memory:")(opts)
			assert.False(t, opts.useFileStorage)
			assert.True(t, opts.useDatabaseStorage)
		})

		t.Run("WithFileSystemStorage clears database storage flag", func(t *testing.T) {
			opts := &runnerOpts{}
			WithDatabaseStorage(":memory:")(opts)
			WithFileSystemStorage(t.TempDir())(opts)
			assert.True(t, opts.useFileStorage)
			assert.False(t, opts.useDatabaseStorage)
		})
	})

	t.Run("AutoMigrate", func(t *testing.T) {
		t.Run("returns nil when using in-memory storage", func(t *testing.T) {
			runner, err := NewRunner(RunnerArgs{
				ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			}, WithLogger(rootTestLogger))
			require.NoError(t, err)
			require.NoError(t, runner.AutoMigrate())
		})

		t.Run("returns nil when using file storage", func(t *testing.T) {
			dir := t.TempDir()
			runner, err := NewRunner(RunnerArgs{
				ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			}, WithLogger(rootTestLogger), WithFileSystemStorage(dir))
			require.NoError(t, err)
			require.NoError(t, runner.AutoMigrate())
		})

		t.Run("succeeds when using database storage", func(t *testing.T) {
			runner, err := NewRunner(RunnerArgs{
				ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			}, WithLogger(rootTestLogger), WithDatabaseStorage(":memory:"))
			require.NoError(t, err)
			require.NoError(t, runner.AutoMigrate())
		})
	})

	t.Run("Run", func(t *testing.T) {
		t.Run("empty sessionID returns error", func(t *testing.T) {
			prov := fake.Lorem().Word()
			mod := fake.Lorem().Word()
			fq := prov + "/" + mod
			providersSvc := lp.NewMockProvidersConfigService(t)
			providersSvc.EXPECT().Get(mock.Anything, prov).Return(&lp.ProviderConfig{
				Name:   prov,
				APIKey: fake.Lorem().Word(),
			}, nil)
			fakeG := internal.NewFakeGenkitInstance()
			runner, err := NewRunner(RunnerArgs{
				ProvidersConfigService: providersSvc,
				genkitInitFunc:         fakeG.InitFunc(),
			}, WithLogger(rootTestLogger))
			require.NoError(t, err)

			_, err = runner.Run(t.Context(), RunParams{
				UserID:    fake.UUID().V4(),
				SessionID: "",
				Model:     fq,
				Message:   &internal.MessageContent{Parts: []internal.MessagePart{{Text: fake.Lorem().Word()}}},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "sessionID")
		})

		t.Run("Model in RunParams propagates to LLMAdapterFactory", func(t *testing.T) {
			modelName := fake.Lorem().Word()
			var capturedModelName string
			factory := internal.NewAgentRunnerFactory(internal.AgentRunnerFactoryDeps{
				LLMAdapterFactory: func(_ context.Context, name string) (adkModel.LLM, error) {
					capturedModelName = name
					return &fakeModel{name: name}, nil
				},
				LLMAgentFactory:       llmagent.New,
				LLMAgentRunnerFactory: internal.RunExecutorFactoryFromRunner,
				SessionStorage:        sessions.NewMemorySessionsStorage(),
				RootLogger:            rootTestLogger,
			})
			r := &Runner{
				runnerFactory: factory,
				toolsProvider: internal.StaticTools(nil),
				rOpts:         &runnerOpts{},
			}
			ctx := t.Context()
			sessionID := fake.UUID().V4()
			userID := fake.UUID().V4()
			msg := &internal.MessageContent{Parts: []internal.MessagePart{{Text: fake.Lorem().Sentence(3)}}}

			result, err := r.Run(ctx, RunParams{
				UserID:    userID,
				SessionID: sessionID,
				Message:   msg,
				Model:     modelName,
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, modelName, capturedModelName)
		})

		t.Run("returns error when model is empty", func(t *testing.T) {
			runner, err := NewRunner(RunnerArgs{
				ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			}, WithLogger(rootTestLogger))
			require.NoError(t, err)

			_, err = runner.Run(t.Context(), RunParams{
				UserID:    fake.UUID().V4(),
				SessionID: fake.UUID().V4(),
				Message:   &internal.MessageContent{Parts: []internal.MessagePart{{Text: fake.Lorem().Word()}}},
			})
			require.Error(t, err)
			require.ErrorContains(t, err, "model is required")
		})

		t.Run("returns error when profileName is provided but profile execution is not configured", func(t *testing.T) {
			runner, err := NewRunner(RunnerArgs{
				ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			}, WithLogger(rootTestLogger))
			require.NoError(t, err)

			_, err = runner.Run(t.Context(), RunParams{
				UserID:      fake.UUID().V4(),
				SessionID:   fake.UUID().V4(),
				Message:     &internal.MessageContent{Parts: []internal.MessagePart{{Text: fake.Lorem().Word()}}},
				ProfileName: "profile-" + fake.Lorem().Word(),
			})
			require.Error(t, err)
			require.ErrorContains(t, err, "profile execution unavailable")
		})

		t.Run("resolves model via ModelsLocator when ProvidersConfigService is set", func(t *testing.T) {
			providerName := fake.Lorem().Word()
			modelName := fake.Lorem().Word()
			fqModel := providerName + "/" + modelName

			providersSvc := lp.NewMockProvidersConfigService(t)
			providersSvc.EXPECT().Get(mock.Anything, providerName).Return(&lp.ProviderConfig{
				Name:   providerName,
				APIKey: fake.Lorem().Word(),
			}, nil)

			fakeG := internal.NewFakeGenkitInstance()
			runner, err := NewRunner(RunnerArgs{
				ProvidersConfigService: providersSvc,
				genkitInitFunc:         fakeG.InitFunc(),
			}, WithLogger(rootTestLogger))
			require.NoError(t, err)

			ctx := t.Context()
			sessionID := fake.UUID().V4()
			userID := fake.UUID().V4()
			msg := &internal.MessageContent{Parts: []internal.MessagePart{{Text: fake.Lorem().Sentence(3)}}}

			result, err := runner.Run(ctx, RunParams{
				UserID:    userID,
				SessionID: sessionID,
				Message:   msg,
				Model:     fqModel,
			})
			require.NoError(t, err)
			require.NotNil(t, result)
		})

		t.Run("profileName dispatches through runner-owned profile execution", func(t *testing.T) {
			providerName := fake.Lorem().Word()
			modelName := fake.Lorem().Word()
			fqModel := providerName + "/" + modelName
			profileName := "profile-" + fake.Lorem().Word()

			providersSvc := lp.NewMockProvidersConfigService(t)
			providersSvc.EXPECT().Get(mock.Anything, providerName).Return(&lp.ProviderConfig{
				Name:   providerName,
				APIKey: fake.Lorem().Word(),
			}, nil)

			profilesSvc := &stubProfilesService{
				get: func(context.Context, string) (*ap.AgentProfile, error) {
					return &ap.AgentProfile{
						Name:         profileName,
						Instructions: fake.Lorem().Sentence(4),
						ExecutionSettings: ap.ExecutionSettings{
							DefaultModel: fqModel,
						},
					}, nil
				},
			}

			fakeG := internal.NewFakeGenkitInstance()
			runner, err := NewRunner(RunnerArgs{
				ProvidersConfigService: providersSvc,
				AgentProfilesService:   profilesSvc,
				genkitInitFunc:         fakeG.InitFunc(),
			}, WithLogger(rootTestLogger))
			require.NoError(t, err)

			result, err := runner.Run(t.Context(), RunParams{
				UserID:      fake.UUID().V4(),
				SessionID:   fake.UUID().V4(),
				Message:     &internal.MessageContent{Parts: []internal.MessagePart{{Text: fake.Lorem().Sentence(3)}}},
				ProfileName: profileName,
			})
			require.NoError(t, err)
			require.NotNil(t, result)
		})

		t.Run("tools registry path still resolves model and runs", func(t *testing.T) {
			providerName := fake.Lorem().Word()
			modelName := fake.Lorem().Word()
			fqModel := providerName + "/" + modelName

			providersSvc := lp.NewMockProvidersConfigService(t)
			providersSvc.EXPECT().Get(mock.Anything, providerName).Return(&lp.ProviderConfig{
				Name:   providerName,
				APIKey: fake.Lorem().Word(),
			}, nil)

			fakeG := internal.NewFakeGenkitInstance()
			runner, err := NewRunner(RunnerArgs{
				ProvidersConfigService: providersSvc,
				genkitInitFunc:         fakeG.InitFunc(),
			},
				WithLogger(rootTestLogger),
				WithToolsRegistry(NewToolsRegistry()),
			)
			require.NoError(t, err)

			result, err := runner.Run(t.Context(), RunParams{
				UserID:    fake.UUID().V4(),
				SessionID: fake.UUID().V4(),
				Message:   &internal.MessageContent{Parts: []internal.MessagePart{{Text: fake.Lorem().Sentence(3)}}},
				Model:     fqModel,
			})
			require.NoError(t, err)
			require.NotNil(t, result)
		})

		t.Run("returns error when model provider is unknown via ModelsLocator", func(t *testing.T) {
			providerName := fake.Lorem().Word()
			modelName := fake.Lorem().Word()
			fqModel := providerName + "/" + modelName

			providersSvc := lp.NewMockProvidersConfigService(t)
			providersSvc.EXPECT().Get(mock.Anything, providerName).Return(nil, errors.New("provider not found"))

			runner, err := NewRunner(RunnerArgs{
				ProvidersConfigService: providersSvc,
			}, WithLogger(rootTestLogger))
			require.NoError(t, err)

			ctx := t.Context()
			_, err = runner.Run(ctx, RunParams{
				UserID:    fake.UUID().V4(),
				SessionID: fake.UUID().V4(),
				Message:   &internal.MessageContent{Parts: []internal.MessagePart{{Text: fake.Lorem().Word()}}},
				Model:     fqModel,
			})
			require.Error(t, err)
			require.ErrorContains(t, err, "provider not found")
		})

		t.Run("valid params returns RunResult", func(t *testing.T) {
			factory := internal.NewAgentRunnerFactory(internal.AgentRunnerFactoryDeps{
				LLMAdapterFactory: func(context.Context, string) (adkModel.LLM, error) {
					return &fakeModel{}, nil
				},
				LLMAgentFactory:       llmagent.New,
				LLMAgentRunnerFactory: internal.RunExecutorFactoryFromRunner,
				SessionStorage:        sessions.NewMemorySessionsStorage(),
				RootLogger:            rootTestLogger,
			})
			r := &Runner{
				runnerFactory: factory,
				toolsProvider: internal.StaticTools(nil),
				rOpts:         &runnerOpts{},
			}
			ctx := t.Context()
			sessionID := fake.UUID().V4()
			userID := fake.UUID().V4()
			msg := &internal.MessageContent{Parts: []internal.MessagePart{{Text: fake.Lorem().Sentence(3)}}}
			modelName := fake.Lorem().Word()

			result, err := r.Run(ctx, RunParams{
				UserID:    userID,
				SessionID: sessionID,
				Message:   msg,
				Model:     modelName,
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, sessionID, result.SessionID())

			got, err := result.ConsumeEventsAsString(ctx)
			require.NoError(t, err)
			assert.Equal(t, "ok", got)
		})
	})

	t.Run("ReadSession", func(t *testing.T) {
		t.Run("returns mapped events from session service", func(t *testing.T) {
			ctx := t.Context()
			stor := sessions.NewMemorySessionsStorage()
			sessionID := fake.UUID().V4()
			userID := fake.UUID().V4()
			text1 := fake.Lorem().Sentence(3)

			createResp, err := stor.Create(ctx, &session.CreateRequest{
				AppName:   defaultRunnerAppName,
				UserID:    userID,
				SessionID: sessionID,
				State:     make(map[string]any),
			})
			require.NoError(t, err)
			sess := createResp.Session

			invocationID := fake.UUID().V4()
			ev1 := newSessionEvent(invocationID, text1)
			require.NoError(t, stor.AppendEvent(ctx, sess, ev1))

			r := &Runner{
				runnerFactory: internal.NewAgentRunnerFactory(internal.AgentRunnerFactoryDeps{
					LLMAdapterFactory: func(context.Context, string) (adkModel.LLM, error) {
						return &fakeModel{}, nil
					},
					LLMAgentFactory:       llmagent.New,
					LLMAgentRunnerFactory: internal.RunExecutorFactoryFromRunner,
					SessionStorage:        stor,
					RootLogger:            rootTestLogger,
				}),
				toolsProvider: internal.StaticTools(nil),
			}

			result, err := r.ReadSession(ctx, ReadSessionParams{
				UserID:    userID,
				SessionID: sessionID,
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, sessionID, result.SessionID())
			assert.False(t, result.IsActive())
			var events []*internal.SessionEvent
			for ev, evErr := range result.Events() {
				require.NoError(t, evErr)
				events = append(events, ev)
			}
			require.Len(t, events, 1)
			require.NotNil(t, events[0].Content)
			assert.Equal(t, text1, events[0].Content.Parts[0].Text)
		})

		t.Run("unknown session returns error", func(t *testing.T) {
			ctx := t.Context()
			r := &Runner{
				runnerFactory: internal.NewAgentRunnerFactory(internal.AgentRunnerFactoryDeps{
					LLMAdapterFactory: func(context.Context, string) (adkModel.LLM, error) {
						return &fakeModel{}, nil
					},
					LLMAgentFactory:       llmagent.New,
					LLMAgentRunnerFactory: internal.RunExecutorFactoryFromRunner,
					SessionStorage:        sessions.NewMemorySessionsStorage(),
					RootLogger:            rootTestLogger,
				}),
				toolsProvider: internal.StaticTools(nil),
			}

			result, err := r.ReadSession(ctx, ReadSessionParams{
				UserID:    fake.UUID().V4(),
				SessionID: fake.UUID().V4(),
			})
			require.Error(t, err)
			assert.Nil(t, result)
		})

		t.Run("uses the same session service instance as Run", func(t *testing.T) {
			ctx := t.Context()
			stor := sessions.NewMemorySessionsStorage()
			r := &Runner{
				runnerFactory: internal.NewAgentRunnerFactory(internal.AgentRunnerFactoryDeps{
					LLMAdapterFactory: func(context.Context, string) (adkModel.LLM, error) {
						return &fakeModel{}, nil
					},
					LLMAgentFactory:       llmagent.New,
					LLMAgentRunnerFactory: internal.RunExecutorFactoryFromRunner,
					SessionStorage:        stor,
					RootLogger:            rootTestLogger,
				}),
				toolsProvider: internal.StaticTools(nil),
				rOpts:         &runnerOpts{},
			}

			sessionID := fake.UUID().V4()
			userID := fake.UUID().V4()
			msg := &internal.MessageContent{Parts: []internal.MessagePart{{Text: fake.Lorem().Sentence(3)}}}
			modelName := fake.Lorem().Word()

			// Run creates a session and populates it
			runResult, err := r.Run(ctx, RunParams{
				UserID:    userID,
				SessionID: sessionID,
				Message:   msg,
				Model:     modelName,
			})
			require.NoError(t, err)
			_, err = runResult.ConsumeEventsAsString(ctx)
			require.NoError(t, err)

			// ReadSession should see the events from the same session service
			readResult, err := r.ReadSession(ctx, ReadSessionParams{
				UserID:    userID,
				SessionID: sessionID,
			})
			require.NoError(t, err)
			require.NotNil(t, readResult)
			assert.Equal(t, sessionID, readResult.SessionID())
			var readEvents []*internal.SessionEvent
			for ev, evErr := range readResult.Events() {
				require.NoError(t, evErr)
				readEvents = append(readEvents, ev)
			}
			assert.NotEmpty(t, readEvents)
		})
	})

	t.Run("ListSessions", func(t *testing.T) {
		t.Run("returns sessions from metadata store", func(t *testing.T) {
			ctx := t.Context()
			userID := fake.UUID().V4()
			sessionID := fake.UUID().V4()
			title := fake.Lorem().Word()
			now := time.Now()

			stor := sessions.NewMemorySessionsStorage()
			require.NoError(t, stor.SaveMetadata(ctx, internal.SessionMetadata{
				SessionID: sessionID,
				AppName:   defaultRunnerAppName,
				UserID:    userID,
				Title:     title,
				CreatedAt: now,
				UpdatedAt: now,
			}))

			r := &Runner{
				runnerFactory: internal.NewAgentRunnerFactory(internal.AgentRunnerFactoryDeps{
					LLMAdapterFactory: func(context.Context, string) (adkModel.LLM, error) {
						return &fakeModel{}, nil
					},
					LLMAgentFactory:       llmagent.New,
					LLMAgentRunnerFactory: internal.RunExecutorFactoryFromRunner,
					SessionStorage:        stor,
					RootLogger:            rootTestLogger,
				}),
				toolsProvider: internal.StaticTools(nil),
				rOpts:         &runnerOpts{},
			}

			result, err := r.ListSessions(ctx, ListSessionsParams{
				UserID: userID,
				Limit:  10,
				Offset: 0,
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, result.Sessions, 1)
			assert.Equal(t, sessionID, result.Sessions[0].SessionID)
			assert.Equal(t, title, result.Sessions[0].Title)
			assert.Equal(t, 1, result.Total)
		})
	})

	t.Run("ModelsLocator", func(t *testing.T) {
		t.Run("returns nil when unset", func(t *testing.T) {
			r := &Runner{}
			assert.Nil(t, r.ModelsLocator())
		})
	})
}

func newSessionEvent(invocationID, text string) *session.Event {
	ev := session.NewEvent(invocationID)
	ev.Content = &genai.Content{Parts: []*genai.Part{{Text: text}}}
	return ev
}

// fakeModel implements model.LLM for tests without a live Genkit provider (same idea as internal tests).
type fakeModel struct{ name string }

func (m *fakeModel) Name() string {
	if m.name != "" {
		return m.name
	}
	return "fake"
}

func (m *fakeModel) GenerateContent(
	_ context.Context,
	_ *adkModel.LLMRequest,
	_ bool,
) iter.Seq2[*adkModel.LLMResponse, error] {
	return func(yield func(*adkModel.LLMResponse, error) bool) {
		yield(&adkModel.LLMResponse{
			Content: &genai.Content{
				Parts: []*genai.Part{{Text: "ok"}},
			},
		}, nil)
	}
}

type stubProfilesService struct {
	get func(ctx context.Context, name string) (*ap.AgentProfile, error)
}

func (s *stubProfilesService) List(context.Context) ([]ap.AgentProfile, error) {
	panic("unexpected List call")
}

func (s *stubProfilesService) Get(ctx context.Context, name string) (*ap.AgentProfile, error) {
	return s.get(ctx, name)
}

func (s *stubProfilesService) Create(context.Context, ap.CreateAgentProfileParams) (*ap.AgentProfile, error) {
	panic("unexpected Create call")
}

func (s *stubProfilesService) Update(
	context.Context,
	string,
	ap.UpdateAgentProfileParams,
) (*ap.AgentProfile, error) {
	panic("unexpected Update call")
}

func (s *stubProfilesService) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func (s *stubProfilesService) AutoMigrate() error {
	panic("unexpected AutoMigrate call")
}
