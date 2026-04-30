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

	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
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

// LLMAgentRunnerRunFactory creates a runner.Runner from runner.Config.
type LLMAgentRunnerRunFactory func(cfg runner.Config) (*runner.Runner, error)

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

type profilesService interface {
	Get(ctx context.Context, name string) (*ap.AgentProfile, error)
}

// ACPRunRequest describes a resolved profile run for ACP stdio execution.
type ACPRunRequest struct {
	ProfileName string
	Profile     *ap.AgentProfile
	Model       string
	UserID      string
	SessionID   string
	Message     *MessageContent
}

// ACPProfileExecutor executes ACP stdio profile runs behind an internal boundary.
type ACPProfileExecutor interface {
	RunACPProfile(ctx context.Context, request ACPRunRequest) (*RunResult, error)
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

type NewAgentRunnerParams struct {
	AppName               string
	AgentName             string
	SystemPromptFragments []SystemPromptFragment
	ToolsRegistry         ToolsProvider
	ModelName             string // from RunParams; public Runner validates non-empty before NewAgentRunner

	// Profile execution fields — required when AgentRunner.Run should handle profile dispatch.
	DefaultAgentName   string
	ProfilesService    profilesService
	ACPProfileExecutor ACPProfileExecutor
}

// NewAgentRunner builds an AgentRunner. When params.ModelName is non-empty the LLM is
// resolved and wired immediately (used for per-request child runners). When ModelName is
// empty no LLM is resolved and the runner acts as a profile dispatcher only — direct runs
// on such a runner must not be attempted.
func (f *AgentRunnerFactory) NewAgentRunner(ctx context.Context, params NewAgentRunnerParams) (*AgentRunner, error) {
	logger := f.rootLogger.With("component", "agent-runner")

	logger.DebugContext(ctx, "Initializing agent runner",
		"appName", params.AppName,
		"agentName", params.AgentName,
		"modelName", params.ModelName,
	)

	toolsProvider := params.ToolsRegistry
	if toolsProvider == nil {
		toolsProvider = StaticTools(nil)
	}

	ar := &AgentRunner{
		factory:               f,
		sessionStorage:        f.sessionStorage,
		appName:               params.AppName,
		defaultAgentName:      params.DefaultAgentName,
		systemPromptFragments: append([]SystemPromptFragment(nil), params.SystemPromptFragments...),
		toolsProvider:         toolsProvider,
		profilesService:       params.ProfilesService,
		acpProfileExecutor:    params.ACPProfileExecutor,
		logger:                logger,
	}

	if params.ModelName == "" {
		return ar, nil
	}

	tools, err := toolsProvider.GetTools()
	if err != nil {
		return nil, fmt.Errorf("build tools: %w", err)
	}

	llmModel, err := f.llmAdapterFactory(ctx, params.ModelName)
	if err != nil {
		return nil, fmt.Errorf("resolve model: %w", err)
	}
	logger.DebugContext(ctx, "Resolved model", "model", llmModel.Name())
	cfg := llmagent.Config{
		Name:                params.AgentName,
		InstructionProvider: newSystemPromptInstructionProvider(params.SystemPromptFragments),
		Tools:               tools,
		Model:               llmModel,
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
	ar.llmRunner = llmRunner
	return ar, nil
}

type AgentRunner struct {
	factory               *AgentRunnerFactory
	sessionStorage        sessions.SessionsStorage
	appName               string
	defaultAgentName      string
	systemPromptFragments []SystemPromptFragment
	toolsProvider         ToolsProvider
	profilesService       profilesService
	acpProfileExecutor    ACPProfileExecutor
	llmRunner             LLMRunner
	logger                *slog.Logger
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

// Run dispatches the run according to profile selection and execution mode.
// Direct runs (no ProfileName) and regular profile runs go through the standard
// built-in agent run path. ACP stdio profiles are delegated to ACPProfileExecutor.
func (a *AgentRunner) Run(ctx context.Context, params RunParams) (*RunResult, error) {
	if params.SessionID == "" {
		return nil, errors.New("sessionID is required")
	}

	profileName := strings.TrimSpace(params.ProfileName)
	requestModel := strings.TrimSpace(params.Model)

	if profileName == "" {
		// If this runner was constructed with a specific model already wired, use it directly.
		if a.llmRunner != nil {
			return a.runWithLLMRunner(ctx, params)
		}
		if requestModel == "" {
			return nil, errors.New("model is required")
		}
		return a.runBuiltIn(ctx, params, a.defaultAgentName, requestModel, "")
	}

	profile, err := a.loadProfile(ctx, profileName)
	if err != nil {
		return nil, err
	}

	switch profile.ExecutionSettings.ModeOrDefault() {
	case ap.ExecutionModeRegular:
		resolvedModel := requestModel
		if resolvedModel == "" {
			resolvedModel = strings.TrimSpace(profile.ExecutionSettings.DefaultModel)
		}
		if resolvedModel == "" {
			return nil, errors.New("model is required")
		}
		return a.runBuiltIn(ctx, params, profile.Name, resolvedModel, profile.Instructions)
	case ap.ExecutionModeACPStdio:
		return a.runACP(ctx, params, profile, requestModel)
	default:
		return nil, WrapAgentExecError(
			AgentExecErrorKindUnsupported,
			"dispatch-profile",
			fmt.Errorf(
				"profile %q uses unsupported execution mode %q",
				profile.Name,
				profile.ExecutionSettings.Mode,
			),
		)
	}
}

func (a *AgentRunner) loadProfile(
	ctx context.Context,
	profileName string,
) (*ap.AgentProfile, error) {
	if a.profilesService == nil {
		return nil, WrapAgentExecError(
			AgentExecErrorKindExecution,
			"load-profile",
			errors.New("profile execution unavailable"),
		)
	}

	profile, err := a.profilesService.Get(ctx, profileName)
	if err != nil {
		if errors.Is(err, ap.ErrAgentProfileNotFound) {
			return nil, WrapAgentExecError(
				AgentExecErrorKindNotFound,
				"load-profile",
				fmt.Errorf("profile %q not found: %w", profileName, err),
			)
		}
		return nil, WrapAgentExecError(
			AgentExecErrorKindExecution,
			"load-profile",
			fmt.Errorf("load profile %q: %w", profileName, err),
		)
	}

	return profile, nil
}

// runWithLLMRunner executes using the already-wired llmRunner on this AgentRunner.
func (a *AgentRunner) runWithLLMRunner(ctx context.Context, params RunParams) (*RunResult, error) {
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

// runBuiltIn creates a child AgentRunner for the given model/agentName/instructions and runs it.
func (a *AgentRunner) runBuiltIn(
	ctx context.Context,
	params RunParams,
	agentName string,
	modelName string,
	profileInstructions string,
) (*RunResult, error) {
	fragments := append([]SystemPromptFragment(nil), a.systemPromptFragments...)
	if instr := strings.TrimSpace(profileInstructions); instr != "" {
		fragments = append(fragments, SystemPromptFragment{
			Section: "Profile Instructions",
			Content: instr,
		})
	}

	child, err := a.factory.NewAgentRunner(ctx, NewAgentRunnerParams{
		AppName:               a.appName,
		AgentName:             agentName,
		SystemPromptFragments: fragments,
		ToolsRegistry:         a.toolsProvider,
		ModelName:             modelName,
	})
	if err != nil {
		return nil, err
	}

	if sessionErr := child.ensureSession(ctx, params.UserID, params.SessionID); sessionErr != nil {
		return nil, sessionErr
	}

	a.logger.DebugContext(ctx,
		"Running agent",
		"userID", params.UserID,
		"sessionID", params.SessionID,
		"message", params.Message,
		"model", modelName,
	)

	genAIMsg := messageContentToGenAI(params.Message)
	adkEvents := child.llmRunner.Run(ctx, params.UserID, params.SessionID, genAIMsg, agent.RunConfig{
		StreamingMode: agent.StreamingModeSSE,
	})
	return NewRunResult(MapADKSessionEventSeq(adkEvents), params.SessionID), nil
}

func (a *AgentRunner) runACP(
	ctx context.Context,
	params RunParams,
	profile *ap.AgentProfile,
	requestModel string,
) (*RunResult, error) {
	if a.acpProfileExecutor == nil {
		return nil, WrapAgentExecError(
			AgentExecErrorKindExecution,
			"run-acp-profile",
			errors.New("ACP profile runner unavailable"),
		)
	}

	return a.acpProfileExecutor.RunACPProfile(ctx, ACPRunRequest{
		ProfileName: profile.Name,
		Profile:     profile,
		Model:       requestModel,
		UserID:      params.UserID,
		SessionID:   params.SessionID,
		Message:     params.Message,
	})
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
) (*runner.Runner, error) {
	return runner.New(cfg)
}
