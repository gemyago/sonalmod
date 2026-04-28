package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/firebase/genkit/go/genkit"
	"github.com/gemyago/sonalmod/runtime/internal"
	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	lp "github.com/gemyago/sonalmod/runtime/internal/llmproviders"
	"github.com/gemyago/sonalmod/runtime/internal/profileexec"
	"github.com/gemyago/sonalmod/runtime/internal/profilerun"
	"github.com/gemyago/sonalmod/runtime/internal/sessions"
	"github.com/gemyago/sonalmod/runtime/internal/summarize"
	"google.golang.org/adk/agent/llmagent"
)

type RunParams = internal.RunParams
type RunResult = internal.RunResult
type ReadSessionParams = internal.ReadSessionParams
type ReadSessionResult = internal.ReadSessionResult

// SessionMetadata is lightweight session listing data (id, title, timestamps).
type SessionMetadata = internal.SessionMetadata

// ListSessionsParams filters and paginates session metadata for the configured app.
type ListSessionsParams = internal.ListSessionMetadataParams

// ListSessionsResult is a page of session metadata plus total count for pagination.
type ListSessionsResult = internal.ListSessionMetadataResult

// SystemPromptFragment is an extra section appended after the runtime base system prompt.
type SystemPromptFragment = internal.SystemPromptFragment

// Compile-time check: toolsRegistryProvider implements internal.ToolsProvider.
var _ internal.ToolsProvider = (*toolsRegistryProvider)(nil)

type RunnerArgs struct {
	// ProvidersConfigService is required. NewRunner wires a ModelsLocator for LLM resolution.
	ProvidersConfigService ProvidersConfigService
	// AgentProfilesService is required. NewRunner wires profile-backed execution from it.
	AgentProfilesService AgentProfilesService

	// genkitInitFunc overrides the genkit initialization function used by ModelsLocator.
	// Intended for tests only; production code leaves this nil.
	genkitInitFunc func(context.Context, lp.ProviderConfig) (*genkit.Genkit, error)
}

type runnerOpts struct {
	logger                *slog.Logger
	toolsRegistry         *ToolsRegistry
	useFileStorage        bool
	fileStorageDir        string
	useDatabaseStorage    bool
	databaseDSN           string
	databaseTablePrefix   string
	systemPromptFragments []SystemPromptFragment
}

type RunnerOpt func(*runnerOpts)

// WithLogger sets the logger to use for the runner.
func WithLogger(logger *slog.Logger) RunnerOpt {
	return func(opts *runnerOpts) {
		opts.logger = logger
	}
}

// WithToolsRegistry sets an optional tools registry. When non-nil, Genkit tool stubs
// are registered on each provider Genkit instance as models are resolved, and ADK tools
// are supplied from the registry.
func WithToolsRegistry(reg *ToolsRegistry) RunnerOpt {
	return func(opts *runnerOpts) {
		opts.toolsRegistry = reg
	}
}

// WithFileSystemStorage configures the runner to persist state under baseDir on disk.
// The directory is created if it does not exist. When this option is omitted,
// storage is in-memory only. If WithFileSystemStorage is passed multiple times,
// the last call wins. WithFileSystemStorage and WithDatabaseStorage are mutually
// exclusive; the last one set wins.
func WithFileSystemStorage(baseDir string) RunnerOpt {
	return func(opts *runnerOpts) {
		opts.useFileStorage = true
		opts.fileStorageDir = baseDir
		opts.useDatabaseStorage = false
		opts.databaseDSN = ""
	}
}

// WithDatabaseStorage configures the runner to persist session state in a database
// identified by dsn. When this option is omitted, storage defaults to in-memory.
// WithDatabaseStorage and WithFileSystemStorage are mutually exclusive; the last
// one set wins.
func WithDatabaseStorage(dsn string) RunnerOpt {
	return func(opts *runnerOpts) {
		opts.useDatabaseStorage = true
		opts.databaseDSN = dsn
		opts.useFileStorage = false
		opts.fileStorageDir = ""
	}
}

// WithDatabaseTablePrefix sets the SQL table name prefix used when database-backed session
// storage is enabled. When empty, no prefix is applied. This option has no effect when storage
// is not database-backed.
func WithDatabaseTablePrefix(prefix string) RunnerOpt {
	return func(opts *runnerOpts) {
		opts.databaseTablePrefix = prefix
	}
}

// WithSystemPromptFragments appends extra system prompt sections after the base template
// and the default assistant instruction fragment. Multiple calls append in order.
func WithSystemPromptFragments(fragments ...SystemPromptFragment) RunnerOpt {
	return func(opts *runnerOpts) {
		opts.systemPromptFragments = append(opts.systemPromptFragments, fragments...)
	}
}

type Runner struct {
	runnerFactory   *internal.AgentRunnerFactory
	toolsProvider   internal.ToolsProvider
	rOpts           *runnerOpts
	sessionsStorage sessions.SessionsStorage
	modelsLocator   *internal.ModelsLocator
	profiles        AgentProfilesService
	profileRuns     *profileexec.Dispatcher
}

func NewRunner(args RunnerArgs, opts ...RunnerOpt) (*Runner, error) {
	if args.ProvidersConfigService == nil {
		return nil, errors.New("providers config service is required")
	}
	if args.AgentProfilesService == nil {
		return nil, errors.New("agent profiles service is required")
	}

	rOpts := &runnerOpts{
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(rOpts)
	}

	var toolsProvider internal.ToolsProvider = internal.StaticTools(nil)
	if rOpts.toolsRegistry != nil {
		toolsProvider = &toolsRegistryProvider{reg: rOpts.toolsRegistry}
	}

	toolStubRegistrar := func(*genkit.Genkit) {}
	if rOpts.toolsRegistry != nil {
		reg := rOpts.toolsRegistry
		toolStubRegistrar = func(g *genkit.Genkit) {
			reg.defineGenkitToolStubs(g)
		}
	}
	modelsLocator := internal.NewModelsLocator(internal.ModelsLocatorParams{
		ProvidersSvc:      args.ProvidersConfigService,
		Logger:            rOpts.logger,
		GenkitInitFunc:    args.genkitInitFunc,
		ToolStubRegistrar: toolStubRegistrar,
	})

	summarizer := summarize.NewLLMSummarizer(
		args.ProvidersConfigService,
		modelsLocator,
		summarize.NewTruncatingSummarizer(),
		rOpts.logger,
	)

	ss, err := sessions.NewSessionsStorage(sessions.SessionServiceFactoryDeps{
		RootLogger:            rOpts.logger,
		UseDatabaseStorage:    rOpts.useDatabaseStorage,
		UseFileStorage:        rOpts.useFileStorage,
		DatabaseDSN:           rOpts.databaseDSN,
		DatabaseTablePrefix:   rOpts.databaseTablePrefix,
		SessionStorageBaseDir: rOpts.fileStorageDir,
		Summarizer:            summarizer,
	})
	if err != nil {
		return nil, fmt.Errorf("session storage: %w", err)
	}

	runnerFactory := internal.NewAgentRunnerFactory(internal.AgentRunnerFactoryDeps{
		LLMAdapterFactory:     modelsLocator.ResolveModel,
		LLMAgentFactory:       llmagent.New,
		LLMAgentRunnerFactory: internal.RunExecutorFactoryFromRunner,
		SessionStorage:        ss,
		RootLogger:            rOpts.logger,
	})

	runner := &Runner{
		runnerFactory:   runnerFactory,
		toolsProvider:   toolsProvider,
		rOpts:           rOpts,
		sessionsStorage: ss,
		modelsLocator:   modelsLocator,
		profiles:        args.AgentProfilesService,
	}

	sessionRecorder, _ := profileexec.NewSessionRecorder(defaultRunnerAppName, ss)
	profileRuns, _ := profileexec.NewDispatcher(args.AgentProfilesService, sessionRecorder)
	runner.profileRuns = profileRuns

	return runner, nil
}

const (
	defaultRunnerAppName   = "sonalmod-runtime"
	defaultRunnerAgentName = "sonalmod"
)

func (r *Runner) newAgentRunnerParams(
	modelName string,
	agentName string,
	profileInstructions string,
) internal.NewAgentRunnerParams {
	systemPromptFragments := append(
		[]SystemPromptFragment(nil),
		r.rOpts.systemPromptFragments...,
	)
	if strings.TrimSpace(profileInstructions) != "" {
		systemPromptFragments = append(systemPromptFragments, SystemPromptFragment{
			Section: "Profile Instructions",
			Content: profileInstructions,
		})
	}

	return internal.NewAgentRunnerParams{
		AppName:               defaultRunnerAppName,
		AgentName:             agentName,
		SystemPromptFragments: systemPromptFragments,
		ToolsRegistry:         r.toolsProvider,
		ModelName:             modelName,
	}
}

// AutoMigrate runs schema migrations for database-backed session storage when configured.
// It is a no-op when file or in-memory storage is in use.
// Call this once after constructing the runner and before serving requests.
func (r *Runner) AutoMigrate() error {
	return r.sessionsStorage.AutoMigrate()
}

// ModelsLocator returns the runner's [ModelsLocator] for listing models (e.g. GET /models).
// The returned value satisfies [ModelsLister] and can be passed to [httpapi.HandlerArgs.ModelsLister].
func (r *Runner) ModelsLocator() ModelsLister { //nolint:ireturn
	if r.modelsLocator == nil {
		return nil
	}
	return r.modelsLocator
}

func (r *Runner) Run(ctx context.Context, params RunParams) (*RunResult, error) {
	profileName := strings.TrimSpace(params.ProfileName)
	modelName := strings.TrimSpace(params.Model)
	if profileName != "" {
		return r.runProfileBackedExecution(ctx, params, profileName, modelName)
	}

	if modelName == "" {
		return nil, errors.New("model is required")
	}
	return r.runBuiltInExecution(ctx, params)
}

func (r *Runner) loadProfile(
	ctx context.Context,
	profileName string,
) (*ap.AgentProfile, error) {
	if r.profiles == nil {
		return nil, profilerun.WrapError(
			profilerun.ErrorKindExecution,
			"load-profile",
			errors.New("profile execution unavailable"),
		)
	}

	profile, err := r.profiles.Get(ctx, profileName)
	if err != nil {
		if errors.Is(err, ap.ErrAgentProfileNotFound) {
			return nil, profilerun.WrapError(
				profilerun.ErrorKindNotFound,
				"load-profile",
				fmt.Errorf("profile %q not found: %w", profileName, err),
			)
		}

		return nil, profilerun.WrapError(
			profilerun.ErrorKindExecution,
			"load-profile",
			fmt.Errorf("load profile %q: %w", profileName, err),
		)
	}

	return profile, nil
}

func (r *Runner) runBuiltInExecution(ctx context.Context, params RunParams) (*RunResult, error) {
	ar, err := r.runnerFactory.NewAgentRunner(
		ctx,
		r.newAgentRunnerParams(params.Model, defaultRunnerAgentName, ""),
	)
	if err != nil {
		return nil, err
	}

	return ar.Run(ctx, params)
}

func (r *Runner) runProfileBackedExecution(
	ctx context.Context,
	params RunParams,
	profileName string,
	requestModel string,
) (*RunResult, error) {
	profile, err := r.loadProfile(ctx, profileName)
	if err != nil {
		return nil, err
	}

	switch profile.ExecutionSettings.ModeOrDefault() {
	case ap.ExecutionModeRegular:
		resolvedModel := strings.TrimSpace(requestModel)
		if resolvedModel == "" {
			resolvedModel = strings.TrimSpace(profile.ExecutionSettings.DefaultModel)
		}
		if resolvedModel == "" {
			return nil, errors.New("model is required")
		}

		return r.runProfileExecution(
			ctx,
			params,
			resolvedModel,
			profile.Name,
			profile.Instructions,
		)
	case ap.ExecutionModeACPStdio:
		if r.profileRuns == nil {
			return nil, profilerun.WrapError(
				profilerun.ErrorKindExecution,
				"dispatch-profile",
				errors.New("profile execution unavailable"),
			)
		}

		return r.profileRuns.Run(ctx, profileexec.RunRequest{
			ProfileName: profileName,
			Profile:     profile,
			Model:       requestModel,
			UserID:      params.UserID,
			SessionID:   params.SessionID,
			Message:     params.Message,
		})
	default:
		return nil, profilerun.WrapError(
			profilerun.ErrorKindUnsupported,
			"dispatch-profile",
			fmt.Errorf(
				"profile %q uses unsupported execution mode %q",
				profile.Name,
				profile.ExecutionSettings.Mode,
			),
		)
	}
}

func (r *Runner) runProfileExecution(
	ctx context.Context,
	params RunParams,
	modelName string,
	agentName string,
	profileInstructions string,
) (*RunResult, error) {
	ar, err := r.runnerFactory.NewAgentRunner(
		ctx,
		r.newAgentRunnerParams(modelName, agentName, profileInstructions),
	)
	if err != nil {
		return nil, err
	}

	return ar.Run(ctx, RunParams{
		UserID:    params.UserID,
		SessionID: params.SessionID,
		Message:   params.Message,
		Model:     modelName,
	})
}

// ReadSession reads the events for a session from the configured session service.
func (r *Runner) ReadSession(ctx context.Context, params ReadSessionParams) (*ReadSessionResult, error) {
	return r.runnerFactory.ReadSession(ctx, internal.ReadSessionParams{
		AppName:   defaultRunnerAppName,
		SessionID: params.SessionID,
		UserID:    params.UserID,
	})
}

// ListSessions returns a page of session metadata for the configured app and user.
// AppName in params is ignored; the runner uses its fixed app name (same as Run and ReadSession).
func (r *Runner) ListSessions(ctx context.Context, params ListSessionsParams) (*ListSessionsResult, error) {
	return r.runnerFactory.ListSessions(ctx, internal.ListSessionMetadataParams{
		AppName: defaultRunnerAppName,
		UserID:  params.UserID,
		Limit:   params.Limit,
		Offset:  params.Offset,
	})
}

// AgentRunner is the embedder-facing contract for agent runs, session reads, and session listing.
// Session listing returns paginated [SessionMetadata] via [ListSessionsResult].
// [*Runner] implements it; internal adapters such as the background runner satisfy it as well.
//
//nolint:revive // name distinguishes this interface from the [*Runner] struct (agent.Runner would stutter worse).
type AgentRunner interface {
	Run(ctx context.Context, params RunParams) (*RunResult, error)
	ReadSession(ctx context.Context, params ReadSessionParams) (*ReadSessionResult, error)
	ListSessions(ctx context.Context, params ListSessionsParams) (*ListSessionsResult, error)
}

var _ AgentRunner = (*Runner)(nil)
