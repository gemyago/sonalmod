package agentapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gemyago/sonalmod/runtime/agent"
)

// ListOpenCodeBindings implements [ServerInterface].
func (s *AgentAPIServer) ListOpenCodeBindings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if s.bindingsSvc == nil {
		writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error", "opencode binding service unavailable")
		return
	}

	bindings, err := s.bindingsSvc.List(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "ListOpenCodeBindings: list", "err", err)
		writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error", "failed to list opencode bindings")
		return
	}

	resp := mapOpenCodeBindingsToResponse(bindings)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// CreateOpenCodeBinding implements [ServerInterface].
func (s *AgentAPIServer) CreateOpenCodeBinding(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if s.bindingsSvc == nil {
		writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error", "opencode binding service unavailable")
		return
	}

	var req CreateOpenCodeBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.DebugContext(ctx, "CreateOpenCodeBinding: decode body", "err", err)
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", "malformed JSON request body")
		return
	}

	cwd := ""
	if req.Cwd != nil {
		cwd = *req.Cwd
	}

	binding, err := s.bindingsSvc.Create(ctx, agent.CreateOpenCodeBindingParams{
		Name:        req.Name,
		ProfileName: req.ProfileName,
		CWD:         cwd,
		AgentCommand: agent.OpenCodeAgentCommand{
			Command: req.AgentCommand.Command,
			Args:    append([]string(nil), req.AgentCommand.Args...),
		},
		LaunchOptions: agent.OpenCodeLaunchOptions{
			Transport: req.LaunchOptions.Transport,
		},
	})
	if err != nil {
		if errors.Is(err, agent.ErrOpenCodeBindingNameConflict) {
			writeProblemDetails(w, http.StatusConflict, "Conflict", "opencode binding with this name already exists")
			return
		}
		s.logger.DebugContext(ctx, "CreateOpenCodeBinding: create", "err", err)
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}

	resp := mapOpenCodeBindingToResponse(*binding)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

// GetOpenCodeBinding implements [ServerInterface].
func (s *AgentAPIServer) GetOpenCodeBinding(w http.ResponseWriter, r *http.Request, bindingName BindingName) {
	ctx := r.Context()
	if s.bindingsSvc == nil {
		writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error", "opencode binding service unavailable")
		return
	}

	binding, err := s.bindingsSvc.Get(ctx, bindingName)
	if err != nil {
		if errors.Is(err, agent.ErrOpenCodeBindingNotFound) {
			writeProblemDetails(w, http.StatusNotFound, "Not Found", "opencode binding not found")
			return
		}
		s.logger.ErrorContext(ctx, "GetOpenCodeBinding: get", "err", err)
		writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error", "failed to get opencode binding")
		return
	}

	resp := mapOpenCodeBindingToResponse(*binding)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// UpdateOpenCodeBinding implements [ServerInterface].
func (s *AgentAPIServer) UpdateOpenCodeBinding(w http.ResponseWriter, r *http.Request, bindingName BindingName) {
	ctx := r.Context()
	if s.bindingsSvc == nil {
		writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error", "opencode binding service unavailable")
		return
	}

	var req UpdateOpenCodeBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.DebugContext(ctx, "UpdateOpenCodeBinding: decode body", "err", err)
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", "malformed JSON request body")
		return
	}

	cwd := ""
	if req.Cwd != nil {
		cwd = *req.Cwd
	}

	binding, err := s.bindingsSvc.Update(ctx, bindingName, agent.UpdateOpenCodeBindingParams{
		CWD: cwd,
		AgentCommand: agent.OpenCodeAgentCommand{
			Command: req.AgentCommand.Command,
			Args:    append([]string(nil), req.AgentCommand.Args...),
		},
		LaunchOptions: agent.OpenCodeLaunchOptions{
			Transport: req.LaunchOptions.Transport,
		},
	})
	if err != nil {
		if errors.Is(err, agent.ErrOpenCodeBindingNotFound) {
			writeProblemDetails(w, http.StatusNotFound, "Not Found", "opencode binding not found")
			return
		}
		s.logger.DebugContext(ctx, "UpdateOpenCodeBinding: update", "err", err)
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}

	resp := mapOpenCodeBindingToResponse(*binding)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// DeleteOpenCodeBinding implements [ServerInterface].
func (s *AgentAPIServer) DeleteOpenCodeBinding(w http.ResponseWriter, r *http.Request, bindingName BindingName) {
	ctx := r.Context()
	if s.bindingsSvc == nil {
		writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error", "opencode binding service unavailable")
		return
	}

	if err := s.bindingsSvc.Delete(ctx, bindingName); err != nil {
		if errors.Is(err, agent.ErrOpenCodeBindingNotFound) {
			writeProblemDetails(w, http.StatusNotFound, "Not Found", "opencode binding not found")
			return
		}
		s.logger.ErrorContext(ctx, "DeleteOpenCodeBinding: delete", "err", err)
		writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error", "failed to delete opencode binding")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
