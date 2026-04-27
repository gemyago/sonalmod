package profileexec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"strings"

	rt "github.com/gemyago/sonalmod/runtime/internal"
	"github.com/gemyago/sonalmod/runtime/internal/codinglane"
)

func newACPStdioRunResult(
	sessionID string,
	result *codinglane.ACPStdioExecutorResult,
) *rt.RunResult {
	events := buildACPStdioSessionEvents(result)
	return rt.NewRunResult(sessionEventSeq(events), sessionID)
}

func newACPStdioRunErrorResult(sessionID string, err error) *rt.RunResult {
	return rt.NewRunResult(sessionEventSeq([]*rt.SessionEvent{acpStdioErrorSessionEvent(err)}), sessionID)
}

func sessionEventSeq(events []*rt.SessionEvent) iter.Seq2[*rt.SessionEvent, error] {
	return func(yield func(*rt.SessionEvent, error) bool) {
		for _, event := range events {
			if !yield(event, nil) {
				return
			}
		}
	}
}

func buildACPStdioSessionEvents(result *codinglane.ACPStdioExecutorResult) []*rt.SessionEvent {
	if result == nil {
		return []*rt.SessionEvent{acpStdioErrorSessionEvent(errors.New("ACP stdio executor returned no result"))}
	}

	events := make([]*rt.SessionEvent, 0, len(result.Updates)+1)
	for _, update := range result.Updates {
		event := mapACPStdioUpdateToSessionEvent(update)
		if event != nil {
			events = append(events, event)
		}
	}

	if len(events) == 0 {
		event := mapPromptResultToSessionEvent(result.PromptResult)
		if event != nil {
			events = append(events, event)
		}
	}

	return events
}

func mapACPStdioUpdateToSessionEvent(update codinglane.ACPStdioUpdate) *rt.SessionEvent {
	if strings.EqualFold(update.Type, "error") {
		return &rt.SessionEvent{
			ErrorCode:    "acp-stdio-error",
			ErrorMessage: firstNonEmpty(acpPayloadText(update.Payload), "ACP stdio error"),
		}
	}

	text := acpPayloadText(update.Payload)
	if text == "" {
		return nil
	}

	return &rt.SessionEvent{
		Partial:      !strings.EqualFold(update.Type, "final"),
		TurnComplete: strings.EqualFold(update.Type, "final"),
		Content: &rt.SessionEventContent{
			Role: "model",
			Parts: []rt.SessionEventPart{{
				Text: text,
			}},
		},
	}
}

func mapPromptResultToSessionEvent(raw json.RawMessage) *rt.SessionEvent {
	text := acpPayloadText(raw)
	if text == "" {
		text = compactJSON(raw)
	}
	if text == "" {
		return nil
	}

	return &rt.SessionEvent{
		TurnComplete: true,
		Content: &rt.SessionEventContent{
			Role: "model",
			Parts: []rt.SessionEventPart{{
				Text: text,
			}},
		},
	}
}

func acpPayloadText(raw json.RawMessage) string {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}

	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}

	return extractACPValueText(payload)
}

func extractACPValueText(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			text := extractACPValueText(item)
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		for _, key := range []string{"message", "text", "content", "result"} {
			text := extractACPValueText(typed[key])
			if text != "" {
				return text
			}
		}
	}

	return ""
}

func compactJSON(raw json.RawMessage) string {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return ""
	}

	var compacted bytes.Buffer
	if err := json.Compact(&compacted, raw); err != nil {
		return strings.TrimSpace(string(raw))
	}

	return compacted.String()
}

func acpStdioErrorSessionEvent(err error) *rt.SessionEvent {
	message := "ACP stdio execution failed"
	code := "acp-stdio-execution"

	var acpErr *codinglane.ACPStdioError
	if errors.As(err, &acpErr) {
		code = "acp-stdio-" + string(acpErr.Kind)
		message = acpStdioErrorMessage(acpErr)
	} else if err != nil {
		message = fmt.Sprintf("ACP stdio execution failed: %s", strings.TrimSpace(err.Error()))
	}

	return &rt.SessionEvent{
		ErrorCode:    code,
		ErrorMessage: message,
	}
}

func acpStdioErrorMessage(err *codinglane.ACPStdioError) string {
	detail := ""
	if err != nil && err.Err != nil {
		detail = strings.TrimSpace(err.Err.Error())
	}

	switch err.Kind {
	case codinglane.ACPStdioErrorKindValidation:
		return firstNonEmpty("ACP stdio request validation failed: "+detail, "ACP stdio request validation failed")
	case codinglane.ACPStdioErrorKindSubprocess:
		return firstNonEmpty("ACP stdio agent failed to start: "+detail, "ACP stdio agent failed to start")
	case codinglane.ACPStdioErrorKindProtocol:
		return firstNonEmpty("ACP stdio protocol error: "+detail, "ACP stdio protocol error")
	default:
		return firstNonEmpty("ACP stdio execution failed: "+detail, "ACP stdio execution failed")
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}

	return ""
}

func messageContentText(message *rt.MessageContent) string {
	if message == nil {
		return ""
	}

	parts := make([]string, 0, len(message.Parts))
	for _, part := range message.Parts {
		text := strings.TrimSpace(part.Text)
		if text != "" {
			parts = append(parts, text)
		}
	}

	return strings.Join(parts, "\n")
}
