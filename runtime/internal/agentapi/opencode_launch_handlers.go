package agentapi

import (
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

	if callerid.FromContext(ctx) == nil {
		writeProblemDetails(w, http.StatusUnauthorized, "Unauthorized", "authentication required")
		return
	}

	var req CreateOpenCodeLaunchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.DebugContext(ctx, "CreateOpenCodeLaunch: decode body", "err", err)
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", "malformed JSON request body")
		return
	}

	profileName := strings.TrimSpace(req.ProfileName)
	if profileName == "" {
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", "profileName is required")
		return
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", "prompt is required")
		return
	}

	launchRequest := agent.OpenCodeLaunchRequest{
		ProfileName: profileName,
		Prompt:      prompt,
	}
	if req.BindingName != nil {
		launchRequest.BindingName = strings.TrimSpace(*req.BindingName)
	}

	result, err := s.launcherSvc.Launch(ctx, launchRequest)
	if err != nil {
		var launchErr *agent.OpenCodeLaunchError
		if errors.As(err, &launchErr) {
			switch launchErr.Kind {
			case agent.OpenCodeLaunchErrorKindValidation:
				writeProblemDetails(w, http.StatusBadRequest, "Bad Request", launchErr.Err.Error())
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
		return
	}

	response := OpenCodeLaunchResponse{
		ProfileName: result.ProfileName,
		BindingName: result.BindingName,
		SessionId:   result.SessionID,
		Updates:     make([]OpenCodeLaunchUpdate, len(result.Updates)),
	}
	if len(result.PromptResult) > 0 {
		raw := map[string]any{}
		if err := json.Unmarshal(result.PromptResult, &raw); err == nil {
			response.PromptResult = raw
		} else {
			response.PromptResult = map[string]any{}
		}
	} else {
		response.PromptResult = map[string]any{}
	}
	for idx, update := range result.Updates {
		payload := map[string]any{}
		if len(update.Payload) > 0 {
			if err := json.Unmarshal(update.Payload, &payload); err != nil {
				payload = map[string]any{}
			}
		}
		response.Updates[idx] = OpenCodeLaunchUpdate{
			SessionId: update.SessionID,
			Type:      update.Type,
			Payload:   payload,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(response)
}
