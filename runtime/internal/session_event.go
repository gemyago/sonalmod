package internal

import (
	"iter"

	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// SessionEventFunctionCall holds the tool invocation requested by the model.
type SessionEventFunctionCall struct {
	ID   string
	Name string
	Args map[string]any
}

// SessionEventFunctionResponse holds the result returned by a tool invocation.
type SessionEventFunctionResponse struct {
	ID       string
	Name     string
	Response map[string]any
}

// SessionEventPart is one segment in streamed session-event content (not agent input; see [MessagePart]).
// Exactly one of Text, FunctionCall, or FunctionResponse is set per part.
type SessionEventPart struct {
	Text             string
	FunctionCall     *SessionEventFunctionCall
	FunctionResponse *SessionEventFunctionResponse
}

// SessionEventContent is a text-only projection of model message content on session events (no genai types).
type SessionEventContent struct {
	Role  string
	Parts []SessionEventPart
}

// SessionEvent is a projection of ADK session.Event fields consumed by RunResult and the agent API stream mapper.
type SessionEvent struct {
	ErrorCode    string
	ErrorMessage string
	Partial      bool
	TurnComplete bool
	Interrupted  bool
	Author       string
	Branch       string
	InvocationID string
	Content      *SessionEventContent
}

func sessionEventContentFromGenAI(c *genai.Content) *SessionEventContent {
	if c == nil {
		return nil
	}
	var parts []SessionEventPart
	for _, p := range c.Parts {
		if p == nil {
			continue
		}
		switch {
		case p.Text != "":
			parts = append(parts, SessionEventPart{Text: p.Text})
		case p.FunctionCall != nil:
			parts = append(parts, SessionEventPart{
				FunctionCall: &SessionEventFunctionCall{
					ID:   p.FunctionCall.ID,
					Name: p.FunctionCall.Name,
					Args: p.FunctionCall.Args,
				},
			})
		case p.FunctionResponse != nil:
			parts = append(parts, SessionEventPart{
				FunctionResponse: &SessionEventFunctionResponse{
					ID:       p.FunctionResponse.ID,
					Name:     p.FunctionResponse.Name,
					Response: p.FunctionResponse.Response,
				},
			})
		}
	}
	return &SessionEventContent{
		Role:  c.Role,
		Parts: parts,
	}
}

// MapADKSessionEvent copies the supported subset from a non-nil ADK event. Nil input yields nil.
func MapADKSessionEvent(ev *session.Event) *SessionEvent {
	if ev == nil {
		return nil
	}
	return &SessionEvent{
		ErrorCode:    ev.ErrorCode,
		ErrorMessage: ev.ErrorMessage,
		Partial:      ev.Partial,
		TurnComplete: ev.TurnComplete,
		Interrupted:  ev.Interrupted,
		Author:       ev.Author,
		Branch:       ev.Branch,
		InvocationID: ev.InvocationID,
		Content:      sessionEventContentFromGenAI(ev.Content),
	}
}

// MapADKSessionEventSeq wraps an ADK event iterator, projecting each event with [MapADKSessionEvent]. Stream errors are forwarded unchanged.
func MapADKSessionEventSeq(seq iter.Seq2[*session.Event, error]) iter.Seq2[*SessionEvent, error] {
	return func(yield func(*SessionEvent, error) bool) {
		for ev, err := range seq {
			if err != nil {
				_ = yield(nil, err)
				return
			}
			if !yield(MapADKSessionEvent(ev), nil) {
				return
			}
		}
	}
}
