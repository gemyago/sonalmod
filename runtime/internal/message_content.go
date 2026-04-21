package internal

import "google.golang.org/genai"

// MessageContent is a minimal representation of an inbound user message.
type MessageContent struct {
	Parts []MessagePart
}

// MessagePart is a single segment of [MessageContent]; only text is supported for now.
type MessagePart struct {
	Text string
}

// messageContentToGenAI maps runtime message content to the GenAI SDK shape for the LLM runner.
// Order of parts is preserved. nil input yields nil.
func messageContentToGenAI(c *MessageContent) *genai.Content {
	if c == nil {
		return nil
	}
	parts := make([]*genai.Part, len(c.Parts))
	for i := range c.Parts {
		parts[i] = &genai.Part{Text: c.Parts[i].Text}
	}
	return &genai.Content{
		Role:  "user",
		Parts: parts,
	}
}
