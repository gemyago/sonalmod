package agentapi

import (
	"errors"
	"maps"
	"strings"

	rt "github.com/gemyago/sonalmod/runtime/internal"
)

// ErrNilSessionEvent is returned when ToStreamEvent is called with a nil *rt.SessionEvent.
var ErrNilSessionEvent = errors.New("agentapi: nil session event")

// AgentAPIStreamEventMapper maps runtime session events to OpenAPI StreamEvent payloads (SSE data JSON).
//
//nolint:revive // name is fixed by API plan (agent-api-start-handler); stutter with package agentapi is intentional.
type AgentAPIStreamEventMapper struct{}

// NewAgentAPIStreamEventMapper returns a stream-event mapper for the SSE driver.
func NewAgentAPIStreamEventMapper() *AgentAPIStreamEventMapper {
	return &AgentAPIStreamEventMapper{}
}

// ToStreamEvent maps a single session event to a StreamEvent union value. It does not write HTTP or SSE frames.
// Events with non-empty error fields map to the error discriminator; others map to agent.
func (m *AgentAPIStreamEventMapper) ToStreamEvent(ev *rt.SessionEvent) (StreamEvent, error) {
	if ev == nil {
		return StreamEvent{}, ErrNilSessionEvent
	}
	if strings.TrimSpace(ev.ErrorCode) != "" || strings.TrimSpace(ev.ErrorMessage) != "" {
		return mapSessionEventToStreamError(ev)
	}
	return mapSessionEventToAgentStreamEvent(ev)
}

func mapSessionEventToStreamError(ev *rt.SessionEvent) (StreamEvent, error) {
	msg := strings.TrimSpace(ev.ErrorMessage)
	if msg == "" {
		msg = strings.TrimSpace(ev.ErrorCode)
	}
	if msg == "" {
		msg = "agent error"
	}
	se := StreamErrorEvent{
		Event:   "error",
		Message: msg,
	}
	if code := strings.TrimSpace(ev.ErrorCode); code != "" {
		se.Code = &code
	}
	var out StreamEvent
	if err := out.FromStreamErrorEvent(se); err != nil {
		return StreamEvent{}, err
	}
	return out, nil
}

func mapSessionEventToAgentStreamEvent(ev *rt.SessionEvent) (StreamEvent, error) {
	agent := AgentStreamEvent{
		Event: "agent",
	}
	if ev.Author != "" {
		a := ev.Author
		agent.Author = &a
	}
	if ev.Branch != "" {
		b := ev.Branch
		agent.Branch = &b
	}
	if ev.InvocationID != "" {
		inv := ev.InvocationID
		agent.InvocationId = &inv
	}
	p := ev.Partial
	agent.Partial = &p
	tc := ev.TurnComplete
	agent.TurnComplete = &tc
	if ev.Interrupted {
		in := true
		agent.Interrupted = &in
	}
	if ac := sessionEventContentToAgentStream(ev.Content); ac != nil {
		agent.Content = ac
	}
	var out StreamEvent
	if err := out.FromAgentStreamEvent(agent); err != nil {
		return StreamEvent{}, err
	}
	return out, nil
}

func sessionEventPartToAgentStream(p rt.SessionEventPart) (AgentStreamPart, bool) {
	switch {
	case p.Text != "":
		text := p.Text
		return AgentStreamPart{Text: &text}, true
	case p.FunctionCall != nil:
		fc := p.FunctionCall
		toolCall := &ToolCallData{Id: fc.ID, Name: fc.Name}
		if len(fc.Args) > 0 {
			args := make(map[string]any, len(fc.Args))
			maps.Copy(args, fc.Args)
			toolCall.Args = &args
		}
		return AgentStreamPart{ToolCall: toolCall}, true
	case p.FunctionResponse != nil:
		fr := p.FunctionResponse
		toolResult := &ToolResultData{Id: fr.ID, Name: fr.Name}
		if len(fr.Response) > 0 {
			resp := make(map[string]any, len(fr.Response))
			maps.Copy(resp, fr.Response)
			toolResult.Response = &resp
		}
		return AgentStreamPart{ToolResult: toolResult}, true
	}
	return AgentStreamPart{}, false
}

func sessionEventContentToAgentStream(c *rt.SessionEventContent) *AgentStreamContent {
	if c == nil {
		return nil
	}
	ac := AgentStreamContent{}
	if c.Role != "" {
		ur := AgentStreamContentRole(c.Role)
		if ur.Valid() {
			r := ur
			ac.Role = &r
		}
	}
	for _, p := range c.Parts {
		if part, ok := sessionEventPartToAgentStream(p); ok {
			ac.Parts = append(ac.Parts, part)
		}
	}
	if len(ac.Parts) == 0 && ac.Role == nil {
		return nil
	}
	return &ac
}
