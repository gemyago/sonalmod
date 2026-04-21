package agentapi

import (
	lp "github.com/gemyago/sonalmod/runtime/internal/llmproviders"
)

const maskAPIKeySuffixLen = 4

// maskAPIKey masks the API key for safe display.
// Returns the last 4 characters prefixed with "..." for keys of 4+ chars,
// "..." for shorter keys, or empty string for empty key.
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) < maskAPIKeySuffixLen {
		return "..."
	}
	return "..." + key[len(key)-maskAPIKeySuffixLen:]
}

// mapModelConfigToAPI converts an llmproviders ModelConfig to the API ModelConfig type.
func mapModelConfigToAPI(m lp.ModelConfig) ModelConfig {
	mc := ModelConfig{Name: m.Name}
	if m.DisplayName != "" {
		mc.DisplayName = &m.DisplayName
	}
	s := m.Summarization
	mc.Summarization = &s
	return mc
}

// mapAPIModelConfigToInternal converts an API ModelConfig to the llmproviders ModelConfig type.
func mapAPIModelConfigToInternal(m ModelConfig) lp.ModelConfig {
	mc := lp.ModelConfig{Name: m.Name}
	if m.DisplayName != nil {
		mc.DisplayName = *m.DisplayName
	}
	if m.Summarization != nil {
		mc.Summarization = *m.Summarization
	}
	return mc
}

// mapProviderConfigToResponse converts an llmproviders ProviderConfig to the API ProviderResponse type.
// The API key is masked — only the last 4 characters are included as apiKeyPreview.
func mapProviderConfigToResponse(cfg lp.ProviderConfig) ProviderResponse {
	resp := ProviderResponse{
		Name:          cfg.Name,
		Type:          cfg.Type,
		BaseUrl:       cfg.BaseURL,
		ApiKeyPreview: maskAPIKey(cfg.APIKey),
		CreatedAt:     cfg.CreatedAt,
		UpdatedAt:     cfg.UpdatedAt,
	}
	if cfg.DisplayName != "" {
		resp.DisplayName = &cfg.DisplayName
	}
	resp.Models = make([]ModelConfig, len(cfg.Models))
	for i, m := range cfg.Models {
		resp.Models[i] = mapModelConfigToAPI(m)
	}
	return resp
}

// mapProviderListToResponse converts a slice of llmproviders ProviderConfig to the API ProviderListResponse type.
func mapProviderListToResponse(configs []lp.ProviderConfig) ProviderListResponse {
	providers := make([]ProviderResponse, len(configs))
	for i, cfg := range configs {
		providers[i] = mapProviderConfigToResponse(cfg)
	}
	return ProviderListResponse{Providers: providers}
}
