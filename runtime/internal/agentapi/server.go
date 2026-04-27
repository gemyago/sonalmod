package agentapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gemyago/sonalmod/runtime/agent"
	rt "github.com/gemyago/sonalmod/runtime/internal"
	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	"github.com/gemyago/sonalmod/runtime/internal/callerid"
	lp "github.com/gemyago/sonalmod/runtime/internal/llmproviders"
	"github.com/gemyago/sonalmod/runtime/internal/profileexec"
)

var _ agent.AgentRunner = (*rt.BackgroundRunner)(nil)

// ModelsLister lists available models across all providers.
type ModelsLister interface {
	ListModels(ctx context.Context) ([]rt.ModelInfo, error)
}

// AgentAPIServer implements [ServerInterface] with injected dependencies.
//
//nolint:revive // name is fixed by API plan (agent-api-start-handler); stutter with package agentapi is intentional.
type AgentAPIServer struct {
	runner       agent.AgentRunner
	profileRuns  profileRunDispatcher
	logger       *slog.Logger
	idGen        IDGen
	reqMap       *AgentAPIRequestMapper
	sse          *AgentAPISSEWriter
	providersSvc lp.ProvidersConfigService
	profilesSvc  ap.AgentProfilesService
	bindingsSvc  agent.OpenCodeBindingService
	launcherSvc  agent.OpenCodeLauncher
	modelsLister ModelsLister
}

var _ ServerInterface = (*AgentAPIServer)(nil)

// agentRunRequestInput holds fields extracted from an agent run HTTP request after
// successful parse, auth, and message mapping. Fields are only valid when
// [AgentAPIServer.parseAgentRunRequest] returns ok true.
type agentRunRequestInput struct {
	Message     *rt.MessageContent
	UserID      string
	Model       string
	ProfileName string
}

type profileRunDispatcher interface {
	Run(ctx context.Context, request profileexec.RunRequest) (*rt.RunResult, error)
}

type agentRunRequestBody struct {
	Message     UserMessageContent `json:"message"`
	Model       *string            `json:"model,omitempty"`
	ProfileName *string            `json:"profileName,omitempty"`
}

// ServerParams holds dependencies for [NewAgentAPIServer].
// Logger must be non-nil; other fields must be non-nil for correct behavior.
// ProvidersConfigService is required.
type ServerParams struct {
	Runner                 agent.AgentRunner
	Logger                 *slog.Logger
	IDGen                  IDGen
	RequestMapper          *AgentAPIRequestMapper
	SSEWriter              *AgentAPISSEWriter
	ProvidersConfigService lp.ProvidersConfigService
	AgentProfilesService   ap.AgentProfilesService
	OpenCodeBindingService agent.OpenCodeBindingService
	OpenCodeLauncher       agent.OpenCodeLauncher
	// ModelsLister is optional; when nil, ListModels returns an empty list.
	ModelsLister ModelsLister
}

// NewAgentAPIServer constructs an [AgentAPIServer] from p.
func NewAgentAPIServer(p ServerParams) *AgentAPIServer {
	var profileRuns profileRunDispatcher
	if p.Runner != nil && p.AgentProfilesService != nil {
		dispatcher, err := profileexec.NewDispatcher(p.AgentProfilesService, p.Runner)
		if err == nil {
			profileRuns = dispatcher
		}
	}

	return &AgentAPIServer{
		runner:       p.Runner,
		profileRuns:  profileRuns,
		logger:       p.Logger,
		idGen:        p.IDGen,
		reqMap:       p.RequestMapper,
		sse:          p.SSEWriter,
		providersSvc: p.ProvidersConfigService,
		profilesSvc:  p.AgentProfilesService,
		bindingsSvc:  p.OpenCodeBindingService,
		launcherSvc:  p.OpenCodeLauncher,
		modelsLister: p.ModelsLister,
	}
}

// StartAgentRun implements [ServerInterface].
func (s *AgentAPIServer) StartAgentRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	in, ok := s.parseAgentRunRequest(w, r, "StartAgentRun")
	if !ok {
		return
	}

	sessionID := s.idGen.MustNewV7().String()
	result, runErr := s.runAgentRequest(ctx, in, sessionID)
	if runErr != nil {
		s.logger.ErrorContext(ctx, "StartAgentRun: runner", "err", runErr)
		writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error", "agent run failed")
		return
	}

	if streamErr := s.sse.StreamAgentRun(ctx, w, result); streamErr != nil {
		s.logger.ErrorContext(ctx, "StartAgentRun: SSE stream", "err", streamErr)
	}
}

// ReadSession implements [ServerInterface].
func (s *AgentAPIServer) ReadSession(
	w http.ResponseWriter, r *http.Request, sessionID SessionId,
) {
	ctx := r.Context()

	id := callerid.FromContext(ctx)
	if id == nil {
		writeProblemDetails(w, http.StatusUnauthorized, "Unauthorized", "authentication required")
		return
	}
	userID := id.UserID()

	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", "sessionId is required")
		return
	}

	output, err := s.runner.ReadSession(ctx, agent.ReadSessionParams{
		SessionID: sid,
		UserID:    userID,
	})
	if err != nil {
		s.logger.DebugContext(ctx, "ReadSession: runner", "err", err)
		writeProblemDetails(w, http.StatusNotFound, "Not Found", "session not found")
		return
	}

	if streamErr := s.sse.StreamSessionRead(ctx, w, output); streamErr != nil {
		s.logger.ErrorContext(ctx, "ReadSession: SSE stream", "err", streamErr)
	}
}

// ContinueAgentRun implements [ServerInterface].
func (s *AgentAPIServer) ContinueAgentRun(w http.ResponseWriter, r *http.Request, sessionID SessionId) {
	ctx := r.Context()

	in, ok := s.parseAgentRunRequest(w, r, "ContinueAgentRun")
	if !ok {
		return
	}

	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", "sessionId is required")
		return
	}

	result, runErr := s.runAgentRequest(ctx, in, sid)
	if runErr != nil {
		s.logger.ErrorContext(ctx, "ContinueAgentRun: runner", "err", runErr)
		writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error", "agent run failed")
		return
	}

	if streamErr := s.sse.StreamAgentRun(ctx, w, result); streamErr != nil {
		s.logger.ErrorContext(ctx, "ContinueAgentRun: SSE stream", "err", streamErr)
	}
}

func (s *AgentAPIServer) parseAgentRunRequest(
	w http.ResponseWriter,
	r *http.Request,
	op string,
) (agentRunRequestInput, bool) {
	ctx := r.Context()

	var req agentRunRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.DebugContext(ctx, op+": decode body", "err", err)
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", "malformed JSON request body")
		return agentRunRequestInput{}, false
	}

	id := callerid.FromContext(ctx)
	if id == nil {
		writeProblemDetails(w, http.StatusUnauthorized, "Unauthorized", "authentication required")
		return agentRunRequestInput{}, false
	}
	userID := id.UserID()

	m, err := s.reqMap.ToMessageContent(req.Message)
	if err != nil {
		s.logger.DebugContext(ctx, op+": map message", "err", err)
		detail := "invalid message content"
		if errors.Is(err, ErrInvalidUserContent) {
			detail = err.Error()
		}
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", detail)
		return agentRunRequestInput{}, false
	}

	model := ""
	if req.Model != nil {
		model = *req.Model
	}

	profileName := ""
	if req.ProfileName != nil {
		profileName = *req.ProfileName
	}

	return agentRunRequestInput{
		Message:     m,
		UserID:      userID,
		Model:       model,
		ProfileName: profileName,
	}, true
}

func (s *AgentAPIServer) runAgentRequest(
	ctx context.Context,
	in agentRunRequestInput,
	sessionID string,
) (*rt.RunResult, error) {
	if strings.TrimSpace(in.ProfileName) != "" && s.profileRuns != nil {
		return s.profileRuns.Run(ctx, profileexec.RunRequest{
			ProfileName: in.ProfileName,
			UserID:      in.UserID,
			SessionID:   sessionID,
			Message:     in.Message,
		})
	}

	return s.runner.Run(ctx, rt.RunParams{
		UserID:    in.UserID,
		SessionID: sessionID,
		Message:   in.Message,
		Model:     in.Model,
	})
}

func writeProblemDetails(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	st := status
	pd := ProblemDetails{
		Title:  &title,
		Detail: &detail,
		Status: &st,
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(pd)
}
