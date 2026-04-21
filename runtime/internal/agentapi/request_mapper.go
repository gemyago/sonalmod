package agentapi

import (
	"errors"
	"fmt"
	"strings"

	rt "github.com/gemyago/sonalmod/runtime/internal"
)

// ErrInvalidUserContent indicates the inbound [UserMessageContent] cannot be turned into [rt.MessageContent] (e.g. empty parts).
var ErrInvalidUserContent = errors.New("agentapi: invalid user message content")

// AgentAPIRequestMapper maps OpenAPI [UserMessageContent] to ADK/genai inputs.
//
//nolint:revive // name is fixed by API plan (agent-api-start-handler); stutter with package agentapi is intentional.
type AgentAPIRequestMapper struct{}

// NewAgentAPIRequestMapper returns a request mapper for StartAgentRun and related handlers.
func NewAgentAPIRequestMapper() *AgentAPIRequestMapper {
	return &AgentAPIRequestMapper{}
}

// ToMessageContent validates and converts API [UserMessageContent] to [*rt.MessageContent].
// Empty or unusable parts return ErrInvalidUserContent (handler maps to 400).
func (m *AgentAPIRequestMapper) ToMessageContent(uc UserMessageContent) (*rt.MessageContent, error) {
	if len(uc.Parts) == 0 {
		return nil, fmt.Errorf("message parts: %w", ErrInvalidUserContent)
	}

	parts := make([]rt.MessagePart, 0, len(uc.Parts))
	for i := range uc.Parts {
		cp := uc.Parts[i]
		t := strings.TrimSpace(cp.Text)
		if t == "" {
			return nil, fmt.Errorf("message part %d: empty text: %w", i, ErrInvalidUserContent)
		}
		parts = append(parts, rt.MessagePart{Text: t})
	}

	return &rt.MessageContent{Parts: parts}, nil
}
