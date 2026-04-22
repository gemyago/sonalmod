package agentapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gemyago/sonalmod/runtime/agent"
	"github.com/gemyago/sonalmod/runtime/internal/callerid"
)

// CreateOpenCodeLaunch implements [ServerInterface].
func (s *AgentAPIServer) CreateOpenCodeLaunch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if s.launcherSvc == nil {
		writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error", "opencode launcher unavailable")
		return
	}
	launchRequest, ok := s.parseOpenCodeLaunchRequest(w, r)
	if !ok {
		return
	}

	result, err := s.launcherSvc.Launch(ctx, launchRequest)
	if err != nil {
		s.writeOpenCodeLaunchError(ctx, w, err)
		return
	}

	response := mapOpenCodeLaunchResponse(*result)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(response)
}

func (s *AgentAPIServer) parseOpenCodeLaunchRequest(
	w http.ResponseWriter,
	r *http.Request,
) (agent.OpenCodeLaunchRequest, bool) {
	if callerid.FromContext(r.Context()) == nil {
		writeProblemDetails(w, http.StatusUnauthorized, "Unauthorized", "authentication required")
		return agent.OpenCodeLaunchRequest{}, false
	}

	var req CreateOpenCodeLaunchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.DebugContext(r.Context(), "CreateOpenCodeLaunch: decode body", "err", err)
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", "malformed JSON request body")
		return agent.OpenCodeLaunchRequest{}, false
	}

	profileName := strings.TrimSpace(req.ProfileName)
	if profileName == "" {
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", "profileName is required")
		return agent.OpenCodeLaunchRequest{}, false
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", "prompt is required")
		return agent.OpenCodeLaunchRequest{}, false
	}

	request := agent.OpenCodeLaunchRequest{
		ProfileName: profileName,
		Prompt:      prompt,
	}
	if req.BindingName != nil {
		request.BindingName = strings.TrimSpace(*req.BindingName)
	}

	return request, true
}

func (s *AgentAPIServer) writeOpenCodeLaunchError(
	ctx context.Context,
	w http.ResponseWriter,
	err error,
) {
	var launchErr *agent.OpenCodeLaunchError
	if errors.As(err, &launchErr) {
		switch launchErr.Kind {
		case agent.OpenCodeLaunchErrorKindValidation:
			detail := "invalid launch request"
			if launchErr.Err != nil {
				detail = launchErr.Err.Error()
			}
			writeProblemDetails(w, http.StatusBadRequest, "Bad Request", detail)
			return
		case agent.OpenCodeLaunchErrorKindNotFound:
			writeProblemDetails(w, http.StatusNotFound, "Not Found", "saved profile or binding not found")
			return
		case agent.OpenCodeLaunchErrorKindLaunchFailed:
			writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error", "opencode launch failed")
			return
		}
	}

	s.logger.ErrorContext(ctx, "CreateOpenCodeLaunch: launch", "err", err)
	writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error", "opencode launch failed")
}

func mapOpenCodeLaunchResponse(result agent.OpenCodeLaunchResult) OpenCodeLaunchResponse {
	response := OpenCodeLaunchResponse{
		ProfileName:  result.ProfileName,
		BindingName:  result.BindingName,
		SessionId:    result.SessionID,
		PromptResult: decodeRawObject(result.PromptResult),
		Updates:      make([]OpenCodeLaunchUpdate, len(result.Updates)),
	}

	for idx, update := range result.Updates {
		response.Updates[idx] = OpenCodeLaunchUpdate{
			SessionId: update.SessionID,
			Type:      update.Type,
			Payload:   decodeRawObject(update.Payload),
		}
	}
	return response
}

func decodeRawObject(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	payload := map[string]any{}
	unmarshalErr := json.Unmarshal(raw, &payload)
	if unmarshalErr != nil {
		return map[string]any{}
	}
	return payload
}
