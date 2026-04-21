package agentapi

import (
	"encoding/json"
	"errors"
	"net/http"

	lp "github.com/gemyago/sonalmod/runtime/internal/llmproviders"
)

// ListModels implements [ServerInterface].
func (s *AgentAPIServer) ListModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if s.modelsLister == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(ModelListResponse{Models: []ModelInfo{}})
		return
	}

	models, err := s.modelsLister.ListModels(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "ListModels: list", "err", err)
		writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error", "failed to list models")
		return
	}

	apiModels := make([]ModelInfo, len(models))
	for i, m := range models {
		info := ModelInfo{
			Provider: m.Provider,
			Name:     m.Name,
		}
		if m.DisplayName != "" {
			info.DisplayName = &m.DisplayName
		}
		apiModels[i] = info
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(ModelListResponse{Models: apiModels})
}

// ListProviders implements [ServerInterface].
func (s *AgentAPIServer) ListProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	configs, err := s.providersSvc.List(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "ListProviders: list", "err", err)
		writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error", "failed to list providers")
		return
	}

	resp := mapProviderListToResponse(configs)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// CreateProvider implements [ServerInterface].
func (s *AgentAPIServer) CreateProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.DebugContext(ctx, "CreateProvider: decode body", "err", err)
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", "malformed JSON request body")
		return
	}

	var displayName string
	if req.DisplayName != nil {
		displayName = *req.DisplayName
	}

	var models []lp.ModelConfig
	if req.Models != nil {
		models = make([]lp.ModelConfig, len(*req.Models))
		for i, m := range *req.Models {
			models[i] = mapAPIModelConfigToInternal(m)
		}
	}

	cfg, err := s.providersSvc.Create(ctx, lp.CreateProviderConfigParams{
		Name:        req.Name,
		Type:        req.Type,
		DisplayName: displayName,
		BaseURL:     req.BaseUrl,
		APIKey:      req.ApiKey,
		Models:      models,
	})
	if err != nil {
		if errors.Is(err, lp.ErrProviderConfigNameConflict) {
			writeProblemDetails(w, http.StatusConflict, "Conflict", "provider with this name already exists")
			return
		}
		s.logger.DebugContext(ctx, "CreateProvider: create", "err", err)
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}

	resp := mapProviderConfigToResponse(*cfg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

// GetProvider implements [ServerInterface].
func (s *AgentAPIServer) GetProvider(w http.ResponseWriter, r *http.Request, providerName ProviderName) {
	ctx := r.Context()

	cfg, err := s.providersSvc.Get(ctx, providerName)
	if err != nil {
		if errors.Is(err, lp.ErrProviderConfigNotFound) {
			writeProblemDetails(w, http.StatusNotFound, "Not Found", "provider not found")
			return
		}
		s.logger.ErrorContext(ctx, "GetProvider: get", "err", err)
		writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error", "failed to get provider")
		return
	}

	resp := mapProviderConfigToResponse(*cfg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// UpdateProvider implements [ServerInterface].
func (s *AgentAPIServer) UpdateProvider(w http.ResponseWriter, r *http.Request, providerName ProviderName) {
	ctx := r.Context()

	var req UpdateProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.DebugContext(ctx, "UpdateProvider: decode body", "err", err)
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", "malformed JSON request body")
		return
	}

	params := lp.UpdateProviderConfigParams{
		BaseURL: req.BaseUrl,
	}
	if req.DisplayName != nil {
		params.DisplayName = *req.DisplayName
	}
	if req.ApiKey != nil {
		params.APIKey = *req.ApiKey
	}
	if req.Models != nil {
		params.Models = make([]lp.ModelConfig, len(*req.Models))
		for i, m := range *req.Models {
			params.Models[i] = mapAPIModelConfigToInternal(m)
		}
	}

	cfg, err := s.providersSvc.Update(ctx, providerName, params)
	if err != nil {
		if errors.Is(err, lp.ErrProviderConfigNotFound) {
			writeProblemDetails(w, http.StatusNotFound, "Not Found", "provider not found")
			return
		}
		s.logger.DebugContext(ctx, "UpdateProvider: update", "err", err)
		writeProblemDetails(w, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}

	resp := mapProviderConfigToResponse(*cfg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// DeleteProvider implements [ServerInterface].
func (s *AgentAPIServer) DeleteProvider(w http.ResponseWriter, r *http.Request, providerName ProviderName) {
	ctx := r.Context()

	if err := s.providersSvc.Delete(ctx, providerName); err != nil {
		if errors.Is(err, lp.ErrProviderConfigNotFound) {
			writeProblemDetails(w, http.StatusNotFound, "Not Found", "provider not found")
			return
		}
		s.logger.ErrorContext(ctx, "DeleteProvider: delete", "err", err)
		writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error", "failed to delete provider")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
