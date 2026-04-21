package agentapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	rt "github.com/gemyago/sonalmod/runtime/internal"
)

// StreamEventMapper maps runtime session events to OpenAPI StreamEvent payloads (used by the SSE driver).
type StreamEventMapper interface {
	ToStreamEvent(ev *rt.SessionEvent) (StreamEvent, error)
}

var _ StreamEventMapper = (*AgentAPIStreamEventMapper)(nil)

// AgentAPISSEWriter streams RunResult as Server-Sent Events (text/event-stream).
//
//nolint:revive // name is fixed by API plan (agent-api-start-handler); stutter with package agentapi is intentional.
type AgentAPISSEWriter struct {
	mapper StreamEventMapper
}

// NewAgentAPISSEWriter returns an SSE response driver with the given stream-event mapper.
func NewAgentAPISSEWriter(mapper StreamEventMapper) *AgentAPISSEWriter {
	return &AgentAPISSEWriter{mapper: mapper}
}

// StreamAgentRun writes the sessionBound event, maps each session event through the injected mapper,
// then writes done. Headers are set for SSE; each logical event is flushed when supported.
func (s *AgentAPISSEWriter) StreamAgentRun(ctx context.Context, w http.ResponseWriter, result *rt.RunResult) error {
	if result == nil {
		return errors.New("agentapi: nil RunResult")
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	var sessionBound StreamEvent
	if err := sessionBound.FromSessionBoundEvent(SessionBoundEvent{
		SessionId: result.SessionID(),
	}); err != nil {
		return err
	}
	if err := writeSSEEvent(w, sessionBound); err != nil {
		return err
	}

	for ev, streamErr := range result.Events() {
		if err := ctx.Err(); err != nil {
			return s.writeStreamError(w, err)
		}
		if streamErr != nil {
			return s.writeStreamError(w, streamErr)
		}
		mapped, mapErr := s.mapper.ToStreamEvent(ev)
		if mapErr != nil {
			return s.writeStreamError(w, mapErr)
		}
		if writeErr := writeSSEEvent(w, mapped); writeErr != nil {
			return writeErr
		}
	}

	var done StreamEvent
	if err := done.FromDoneEvent(DoneEvent{}); err != nil {
		return err
	}
	return writeSSEEvent(w, done)
}

// StreamSessionRead writes the sessionBound event, sessionStatus (active/idle), maps each session
// event through the injected mapper, then writes done. Headers are set for SSE; each logical event
// is flushed when supported. The unified Events() iterator in output handles pre-run history and
// current-run events seamlessly — the SSE writer does not need to distinguish between them.
func (s *AgentAPISSEWriter) StreamSessionRead(
	ctx context.Context, w http.ResponseWriter, output *rt.ReadSessionResult,
) error {
	if output == nil {
		return errors.New("agentapi: nil ReadSessionResult")
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	var sessionBound StreamEvent
	if err := sessionBound.FromSessionBoundEvent(SessionBoundEvent{
		SessionId: output.SessionID(),
	}); err != nil {
		return err
	}
	if err := writeSSEEvent(w, sessionBound); err != nil {
		return err
	}

	status := Idle
	if output.IsActive() {
		status = Active
	}
	var sessionStatus StreamEvent
	if err := sessionStatus.FromSessionStatusEvent(SessionStatusEvent{
		Status: status,
	}); err != nil {
		return err
	}
	if err := writeSSEEvent(w, sessionStatus); err != nil {
		return err
	}

	for ev, streamErr := range output.Events() {
		if err := ctx.Err(); err != nil {
			return s.writeStreamError(w, err)
		}
		if streamErr != nil {
			return s.writeStreamError(w, streamErr)
		}
		mapped, mapErr := s.mapper.ToStreamEvent(ev)
		if mapErr != nil {
			return s.writeStreamError(w, mapErr)
		}
		if writeErr := writeSSEEvent(w, mapped); writeErr != nil {
			return writeErr
		}
	}

	var done StreamEvent
	if err := done.FromDoneEvent(DoneEvent{}); err != nil {
		return err
	}
	return writeSSEEvent(w, done)
}

func (s *AgentAPISSEWriter) writeStreamError(w http.ResponseWriter, err error) error {
	var se StreamEvent
	msg := err.Error()
	if msg == "" {
		msg = "stream error"
	}
	if e := se.FromStreamErrorEvent(StreamErrorEvent{Event: "error", Message: msg}); e != nil {
		return e
	}
	_ = writeSSEEvent(w, se)
	return err
}

func writeSSEEvent(w http.ResponseWriter, se StreamEvent) error {
	raw, err := json.Marshal(se)
	if err != nil {
		return err
	}
	d, err := se.Discriminator()
	if err != nil {
		return err
	}
	if _, werr := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", d, raw); werr != nil {
		return werr
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}
