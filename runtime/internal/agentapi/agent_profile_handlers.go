package agentapi

import (
	"encoding/json"
	"errors"
	"net/http"

	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
)

// ListAgentProfiles implements [ServerInterface].
func (s *AgentAPIServer) ListAgentProfiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	profiles, err := s.profilesSvc.List(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "ListAgentProfiles: list", "err", err)
		writeProblemDetails(
			w,
			http.StatusInternalServerError,
			"Internal Server Error",
			"failed to list agent profiles",
		)
		return
	}

	resp := mapAgentProfilesToResponse(profiles)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// CreateAgentProfile implements [ServerInterface].
func (s *AgentAPIServer) CreateAgentProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req createAgentProfileRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.DebugContext(ctx, "CreateAgentProfile: decode body", "err", err)
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", "malformed JSON request body")
		return
	}

	params := ap.CreateAgentProfileParams{
		Name:              req.Name,
		Role:              req.Role,
		Instructions:      req.Instructions,
		ExecutionSettings: mapExecutionSettingsToInternal(req.ExecutionSettings),
	}
	if req.DisplayName != nil {
		params.DisplayName = *req.DisplayName
	}
	if req.ToolRefs != nil {
		params.ToolRefs = append([]string(nil), *req.ToolRefs...)
	}

	profile, err := s.profilesSvc.Create(ctx, params)
	if err != nil {
		if errors.Is(err, ap.ErrAgentProfileNameConflict) {
			writeProblemDetails(w, http.StatusConflict, "Conflict", "agent profile with this name already exists")
			return
		}
		s.logger.DebugContext(ctx, "CreateAgentProfile: create", "err", err)
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}

	resp := mapAgentProfileToResponse(*profile)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

// GetAgentProfile implements [ServerInterface].
func (s *AgentAPIServer) GetAgentProfile(w http.ResponseWriter, r *http.Request, profileName ProfileName) {
	ctx := r.Context()

	profile, err := s.profilesSvc.Get(ctx, profileName)
	if err != nil {
		if errors.Is(err, ap.ErrAgentProfileNotFound) {
			writeProblemDetails(w, http.StatusNotFound, "Not Found", "agent profile not found")
			return
		}
		s.logger.ErrorContext(ctx, "GetAgentProfile: get", "err", err)
		writeProblemDetails(
			w,
			http.StatusInternalServerError,
			"Internal Server Error",
			"failed to get agent profile",
		)
		return
	}

	resp := mapAgentProfileToResponse(*profile)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// UpdateAgentProfile implements [ServerInterface].
func (s *AgentAPIServer) UpdateAgentProfile(w http.ResponseWriter, r *http.Request, profileName ProfileName) {
	ctx := r.Context()

	var req updateAgentProfileRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.DebugContext(ctx, "UpdateAgentProfile: decode body", "err", err)
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", "malformed JSON request body")
		return
	}

	params := ap.UpdateAgentProfileParams{
		Role:              req.Role,
		Instructions:      req.Instructions,
		ExecutionSettings: mapExecutionSettingsToInternal(req.ExecutionSettings),
	}
	if req.DisplayName != nil {
		params.DisplayName = *req.DisplayName
	}
	if req.ToolRefs != nil {
		params.ToolRefs = append([]string(nil), *req.ToolRefs...)
	}

	profile, err := s.profilesSvc.Update(ctx, profileName, params)
	if err != nil {
		if errors.Is(err, ap.ErrAgentProfileNotFound) {
			writeProblemDetails(w, http.StatusNotFound, "Not Found", "agent profile not found")
			return
		}
		s.logger.DebugContext(ctx, "UpdateAgentProfile: update", "err", err)
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}

	resp := mapAgentProfileToResponse(*profile)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// DeleteAgentProfile implements [ServerInterface].
func (s *AgentAPIServer) DeleteAgentProfile(w http.ResponseWriter, r *http.Request, profileName ProfileName) {
	ctx := r.Context()

	if err := s.profilesSvc.Delete(ctx, profileName); err != nil {
		if errors.Is(err, ap.ErrAgentProfileNotFound) {
			writeProblemDetails(w, http.StatusNotFound, "Not Found", "agent profile not found")
			return
		}
		s.logger.ErrorContext(ctx, "DeleteAgentProfile: delete", "err", err)
		writeProblemDetails(
			w,
			http.StatusInternalServerError,
			"Internal Server Error",
			"failed to delete agent profile",
		)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
