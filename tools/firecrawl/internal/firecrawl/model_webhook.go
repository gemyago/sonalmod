package firecrawl

type Webhook struct {
	URL      string            `json:"url"`
	Headers  map[string]string `json:"headers,omitempty"`
	Metadata map[string]any    `json:"metadata,omitempty"`
	Events   []string          `json:"events,omitempty"`
}
