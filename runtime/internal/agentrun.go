package internal

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"strings"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"

	"github.com/gemyago/sonalmod/runtime/internal/sessions"
)

// LLMAdapterFactory creates a model.LLM from a model name. Callers may pass an empty name
// when the factory ignores it (e.g. tests); the exported agent.Runner requires a non-empty
// RunParams.Model and does not substitute a runner-level default.
// The context MUST be the same as the run (e.g. passed to NewAgentRunner) so cancellation
// and request-scoped values propagate to model resolution.
// On failure the LLM must be nil and err non-nil (never return a nil LLM without an error).
type LLMAdapterFactory func(ctx context.Context, modelName string) (model.LLM, error)

// LLMAgentFactory creates an agent.Agent from a llmagent.Config.
type LLMAgentFactory func(cfg llmagent.Config) (agent.Agent, error)

// LLMAgentRunnerRunFactory creates a LLMRunner from runner.Config.
type LLMAgentRunnerRunFactory func(cfg runner.Config) (LLMRunner, error)

// LLMRunner executes an agent run and yields session events.
// Compatible with runner.Runner.Run; *runner.Runner implements this interface directly.
type LLMRunner interface {
	Run(
		ctx context.Context,
		userID, sessionID string,
		msg *genai.Content,
		cfg agent.RunConfig,
		opts ...runner.RunOption,
	) iter.Seq2[*session.Event, error]
}

type AgentRunnerFactory struct {
	llmAdapterFactory     LLMAdapterFactory
	llmAgentFactory       LLMAgentFactory
	llmAgentRunnerFactory LLMAgentRunnerRunFactory
	sessionStorage        sessions.SessionsStorage
	rootLogger            *slog.Logger
}

type AgentRunnerFactoryDeps struct {
	LLMAdapterFactory     LLMAdapterFactory
	LLMAgentFactory       LLMAgentFactory
	LLMAgentRunnerFactory LLMAgentRunnerRunFactory
	SessionStorage        sessions.SessionsStorage
	RootLogger            *slog.Logger
}

func NewAgentRunnerFactory(deps AgentRunnerFactoryDeps) *AgentRunnerFactory {
	return &AgentRunnerFactory{
		llmAdapterFactory:     deps.LLMAdapterFactory,
		llmAgentFactory:       deps.LLMAgentFactory,
		llmAgentRunnerFactory: deps.LLMAgentRunnerFactory,
		sessionStorage:        deps.SessionStorage,
		rootLogger:            deps.RootLogger,
	}
}

// ToolsProvider supplies tools for an agent. Implemented by *aitools.ToolsRegistry.
type ToolsProvider interface {
	GetTools() ([]tool.Tool, error)
}

// StaticTools returns a provider that serves a fixed set of tools.
func StaticTools(tools []tool.Tool) *StaticToolsProvider {
	return &StaticToolsProvider{tools: tools}
}

// StaticToolsProvider returns a fixed set of tools.
type StaticToolsProvider struct {
	tools []tool.Tool
}

func (s *StaticToolsProvider) GetTools() ([]tool.Tool, error) {
	return s.tools, nil
}

type NewAgentRunnerParams struct {
	AppName               string
	AgentName             string
	SystemPromptFragments []SystemPromptFragment
	ToolsRegistry         ToolsProvider
	ModelName             string // from RunParams; public Runner validates non-empty before NewAgentRunner
}

func (f *AgentRunnerFactory) NewAgentRunner(ctx context.Context, params NewAgentRunnerParams) (*AgentRunner, error) {
	logger := f.rootLogger.With("component", "agent-runner")

	logger.DebugContext(ctx, "Initializing agent runner",
		"appName", params.AppName,
		"agentName", params.AgentName,
		"modelName", params.ModelName,
	)

	tools, err := params.ToolsRegistry.GetTools()
	if err != nil {
		return nil, fmt.Errorf("build tools: %w", err)
	}

	model, err := f.llmAdapterFactory(ctx, params.ModelName)
	if err != nil {
		return nil, fmt.Errorf("resolve model: %w", err)
	}
	logger.DebugContext(ctx, "Resolved model", "model", model.Name())
	cfg := llmagent.Config{
		Name:                params.AgentName,
		InstructionProvider: newSystemPromptInstructionProvider(params.SystemPromptFragments),
		Tools:               tools,
		Model:               model,
	}
	ag, err := f.llmAgentFactory(cfg)
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}
	llmRunner, err := f.llmAgentRunnerFactory(runner.Config{
		AppName:        params.AppName,
		Agent:          ag,
		SessionService: f.sessionStorage,
	})
	if err != nil {
		return nil, fmt.Errorf("create run executor: %w", err)
	}
	return &AgentRunner{
		sessionStorage: f.sessionStorage,
		appName:        params.AppName,
		llmRunner:      llmRunner,
		logger:         logger,
	}, nil
}

type AgentRunner struct {
	sessionStorage sessions.SessionsStorage
	appName        string
	llmRunner      LLMRunner
	logger         *slog.Logger
}

// ensureSession verifies the session exists for the given sessionID.
// Requires non-empty sessionID. Tries Get first; if not found, creates via Create.
func (a *AgentRunner) ensureSession(
	ctx context.Context,
	userID, sessionID string,
) error {
	if sessionID == "" {
		return errors.New("sessionID is required")
	}

	_, err := a.sessionStorage.Get(ctx, &session.GetRequest{
		AppName:   a.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err == nil {
		return nil
	}
	if !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("session %s: %w", sessionID, err)
	}

	_, err = a.sessionStorage.Create(ctx, &session.CreateRequest{
		AppName:   a.appName,
		UserID:    userID,
		SessionID: sessionID,
		State:     make(map[string]any),
	})
	if err != nil {
		return fmt.Errorf("create session %s: %w", sessionID, err)
	}
	return nil
}

type RunParams struct {
	UserID    string
	SessionID string
	Message   *MessageContent
	Model     string // fully qualified: "provider/model-name"
	// ProfileName selects a saved profile for profile-backed execution.
	// Empty means direct built-in execution using Model.
	ProfileName string
}

func (a *AgentRunner) Run(ctx context.Context, params RunParams) (*RunResult, error) {
	if params.SessionID == "" {
		return nil, errors.New("sessionID is required")
	}

	if err := a.ensureSession(ctx, params.UserID, params.SessionID); err != nil {
		return nil, err
	}

	a.logger.DebugContext(ctx,
		"Running agent",
		"userID", params.UserID,
		"sessionID", params.SessionID,
		"message", params.Message,
		"model", params.Model,
	)

	genAIMsg := messageContentToGenAI(params.Message)
	adkEvents := a.llmRunner.Run(ctx, params.UserID, params.SessionID, genAIMsg, agent.RunConfig{
		StreamingMode: agent.StreamingModeSSE,
	})
	return NewRunResult(MapADKSessionEventSeq(adkEvents), params.SessionID), nil
}

// ReadSessionParams contains the parameters for reading a session.
type ReadSessionParams struct {
	AppName   string
	SessionID string
	UserID    string
}

// ReadSessionResult is the result of reading a session: identifier, whether a background run is active
// (only when using BackgroundRunner), and a replayable event stream.
type ReadSessionResult struct {
	sessionID string
	isActive  bool
	events    iter.Seq2[*SessionEvent, error]
}

// NewReadSessionResult constructs a ReadSessionResult. Storage-only reads use isActive false.
func NewReadSessionResult(sessionID string, isActive bool, events iter.Seq2[*SessionEvent, error]) *ReadSessionResult {
	return &ReadSessionResult{sessionID: sessionID, isActive: isActive, events: events}
}

// SessionID returns the session identifier.
func (r *ReadSessionResult) SessionID() string { return r.sessionID }

// IsActive reports whether a background run is in progress for this session (BackgroundRunner only).
func (r *ReadSessionResult) IsActive() bool { return r.isActive }

// Events returns the session event stream (historical and, when active, live events).
func (r *ReadSessionResult) Events() iter.Seq2[*SessionEvent, error] { return r.events }

// ListSessions returns a page of session metadata from the configured metadata store.
func (f *AgentRunnerFactory) ListSessions(
	ctx context.Context,
	params ListSessionMetadataParams,
) (*ListSessionMetadataResult, error) {
	return f.sessionStorage.ListMetadata(ctx, params)
}

// ListSessions returns a page of session metadata for this runner's app name.
// When no metadata store is configured, it returns an empty page.
func (a *AgentRunner) ListSessions(
	ctx context.Context,
	params ListSessionMetadataParams,
) (*ListSessionMetadataResult, error) {
	if a.sessionStorage == nil {
		return &ListSessionMetadataResult{}, nil
	}
	p := params
	p.AppName = a.appName
	return a.sessionStorage.ListMetadata(ctx, p)
}

// ReadSession reads session events from the configured session service and maps them to SessionEvent.
func (f *AgentRunnerFactory) ReadSession(ctx context.Context, params ReadSessionParams) (*ReadSessionResult, error) {
	resp, err := f.sessionStorage.Get(ctx, &session.GetRequest{
		AppName:   params.AppName,
		UserID:    params.UserID,
		SessionID: params.SessionID,
	})
	if err != nil {
		return nil, err
	}
	var events []*SessionEvent
	for ev := range resp.Session.Events().All() {
		events = append(events, MapADKSessionEvent(ev))
	}
	return NewReadSessionResult(params.SessionID, false, sliceToIter(events)), nil
}

// ReadSession reads session events from the configured session service and maps them to SessionEvent.
// It uses the runner's own session service and app name (same as Run), so it does not require a factory.
func (a *AgentRunner) ReadSession(ctx context.Context, params ReadSessionParams) (*ReadSessionResult, error) {
	resp, err := a.sessionStorage.Get(ctx, &session.GetRequest{
		AppName:   a.appName,
		UserID:    params.UserID,
		SessionID: params.SessionID,
	})
	if err != nil {
		return nil, err
	}
	var events []*SessionEvent
	for ev := range resp.Session.Events().All() {
		events = append(events, MapADKSessionEvent(ev))
	}
	return NewReadSessionResult(params.SessionID, false, sliceToIter(events)), nil
}

// sliceToIter converts a materialized slice to a replayable iterator.
func sliceToIter(events []*SessionEvent) iter.Seq2[*SessionEvent, error] {
	return func(yield func(*SessionEvent, error) bool) {
		for _, ev := range events {
			if !yield(ev, nil) {
				return
			}
		}
	}
}

// RunExecutorFactoryFromRunner adapts [runner.New] to [LLMAgentRunnerRunFactory].
func RunExecutorFactoryFromRunner(
	cfg runner.Config,
) (LLMRunner, error) {
	return runner.New(cfg)
}
