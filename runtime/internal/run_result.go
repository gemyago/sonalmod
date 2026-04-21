package internal

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
)

type RunResult struct {
	events    iter.Seq2[*SessionEvent, error]
	sessionID string
}

func NewRunResult(events iter.Seq2[*SessionEvent, error], sessionID string) *RunResult {
	return &RunResult{events: events, sessionID: sessionID}
}

func (r *RunResult) SessionID() string {
	return r.sessionID
}

// Events returns the session event stream for this run—the same sequence passed to NewRunResult.
func (r *RunResult) Events() iter.Seq2[*SessionEvent, error] {
	return r.events
}

// ResultSelection controls how multiple final (or partial) events with content are combined.
type ResultSelection int

const (
	ResultSelectionConcatenateAll ResultSelection = iota
	ResultSelectionLastOnly
)

// EmptyResultBehavior controls what happens when no content is found in the event stream.
type EmptyResultBehavior int

const (
	EmptyResultBehaviorReturnError EmptyResultBehavior = iota
	EmptyResultBehaviorReturnEmpty
)

// ConsumeOption is a functional option for ConsumeEventsAsString.
type ConsumeOption func(*consumeOptions)

type consumeOptions struct {
	resultSelection     ResultSelection
	emptyResultBehavior EmptyResultBehavior
	agentName           string
}

// WithResultSelection sets how multiple events with content are combined.
func WithResultSelection(s ResultSelection) ConsumeOption {
	return func(o *consumeOptions) { o.resultSelection = s }
}

// WithEmptyResultBehavior sets what happens when no content is found.
func WithEmptyResultBehavior(b EmptyResultBehavior) ConsumeOption {
	return func(o *consumeOptions) { o.emptyResultBehavior = b }
}

// WithAgentName sets the agent name for error messages.
func WithAgentName(name string) ConsumeOption {
	return func(o *consumeOptions) { o.agentName = name }
}

func applyConsumeOptions(opts []ConsumeOption) consumeOptions {
	o := consumeOptions{
		resultSelection:     ResultSelectionConcatenateAll,
		emptyResultBehavior: EmptyResultBehaviorReturnError,
		agentName:           "",
	}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

func (r *RunResult) ConsumeEventsAsString(ctx context.Context, opts ...ConsumeOption) (string, error) {
	o := applyConsumeOptions(opts)
	return r.consumeEventsAsStringImpl(ctx, o)
}

func emptyResultErr(o consumeOptions) error {
	if o.emptyResultBehavior == EmptyResultBehaviorReturnEmpty {
		return nil
	}
	return errors.New("no final answer found in event stream")
}

func formatStreamError(o consumeOptions, err error) error {
	if o.agentName != "" {
		return fmt.Errorf("sub-agent %q: error in event stream: %w", o.agentName, err)
	}
	return fmt.Errorf("error in event stream: %w", err)
}

func formatEventError(o consumeOptions, code, message string) error {
	if o.agentName != "" {
		return fmt.Errorf("sub-agent %q error (code: %q, message: %q)",
			o.agentName, code, message)
	}
	return fmt.Errorf("agent error (code: %q, message: %q)", code, message)
}

func (r *RunResult) consumeEventsAsStringImpl(ctx context.Context, o consumeOptions) (string, error) {
	finalChunks := make([]string, 0)
	partialChunks := make([]string, 0)
	var lastFinalText, lastPartialText string

	for event, streamErr := range r.events {
		if streamErr != nil {
			return "", formatStreamError(o, streamErr)
		}

		if event != nil && (event.ErrorCode != "" || event.ErrorMessage != "") {
			return "", formatEventError(o, event.ErrorCode, event.ErrorMessage)
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}

		chunks := eventTextParts(event)
		if len(chunks) == 0 {
			continue
		}

		eventText := strings.Join(chunks, "\n")
		if event.Partial {
			partialChunks = append(partialChunks, chunks...)
			lastPartialText = eventText
			continue
		}

		finalChunks = append(finalChunks, chunks...)
		lastFinalText = eventText
	}

	if o.resultSelection == ResultSelectionLastOnly {
		if lastFinalText != "" {
			return lastFinalText, nil
		}
		if lastPartialText != "" {
			return lastPartialText, nil
		}
		return "", emptyResultErr(o)
	}

	if len(finalChunks) > 0 {
		return strings.Join(finalChunks, "\n"), nil
	}

	if len(partialChunks) > 0 {
		return strings.Join(partialChunks, "\n"), nil
	}

	return "", emptyResultErr(o)
}

func (r *RunResult) ConsumeEventsAsStringSeq(ctx context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		sawPartialWithText := false

		for event, streamErr := range r.events {
			var keepStreaming bool
			sawPartialWithText, keepStreaming = processStreamingEvent(
				ctx,
				event,
				streamErr,
				sawPartialWithText,
				yield,
			)
			if !keepStreaming {
				return
			}
		}
	}
}

func processStreamingEvent(
	ctx context.Context,
	event *SessionEvent,
	streamErr error,
	sawPartialWithText bool,
	yield func(string, error) bool,
) (bool, bool) {
	if streamErr != nil {
		yield("", fmt.Errorf("error in event stream: %w", streamErr))
		return sawPartialWithText, false
	}

	if event != nil && (event.ErrorCode != "" || event.ErrorMessage != "") {
		yield("", fmt.Errorf("agent error (code: %q, message: %q)", event.ErrorCode, event.ErrorMessage))
		return sawPartialWithText, false
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		yield("", ctxErr)
		return sawPartialWithText, false
	}

	chunks := eventTextParts(event)
	if len(chunks) == 0 {
		return sawPartialWithText, true
	}

	if shouldSkipStreamingEvent(event, sawPartialWithText) {
		return sawPartialWithText, true
	}

	if event.Partial {
		sawPartialWithText = true
	}

	for _, chunk := range chunks {
		if !yield(chunk, nil) {
			return sawPartialWithText, false
		}
	}

	return sawPartialWithText, true
}

func shouldSkipStreamingEvent(event *SessionEvent, sawPartialWithText bool) bool {
	if event == nil {
		return false
	}

	// When partial text has already been streamed, final non-partial text is usually
	// the full aggregated answer and would duplicate what was already printed.
	return !event.Partial && sawPartialWithText
}

func eventTextParts(event *SessionEvent) []string {
	if event == nil {
		return nil
	}

	return contentTextParts(event.Content)
}

func contentTextParts(content *SessionEventContent) []string {
	if content == nil {
		return nil
	}

	chunks := make([]string, 0, len(content.Parts))
	for _, part := range content.Parts {
		if part.Text == "" {
			continue
		}
		chunks = append(chunks, part.Text)
	}

	return chunks
}
