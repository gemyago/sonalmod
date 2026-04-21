package agent

import "github.com/gemyago/sonalmod/runtime/internal"

// RunParamsBuilder constructs [RunParams] for [Runner.Run] with required user identity and model.
type RunParamsBuilder struct {
	userID    string
	sessionID string
	model     string
}

// NewRunParams starts a builder with userID, sessionID, and fully qualified model ("provider/model-name").
func NewRunParams(userID, sessionID, model string) RunParamsBuilder {
	return RunParamsBuilder{
		userID:    userID,
		sessionID: sessionID,
		model:     model,
	}
}

// WithText sets the user message to a single text part and returns the final [RunParams].
func (b RunParamsBuilder) WithText(text string) RunParams {
	return RunParams{
		UserID:    b.userID,
		SessionID: b.sessionID,
		Model:     b.model,
		Message:   &internal.MessageContent{Parts: []internal.MessagePart{{Text: text}}},
	}
}
