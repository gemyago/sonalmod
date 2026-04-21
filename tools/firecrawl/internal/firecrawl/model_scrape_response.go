package firecrawl

// ScrapeResponse is the top-level response returned from the /scrape endpoint.
type ScrapeResponse struct {
	Success bool        `json:"success"`
	Data    *ScrapeData `json:"data,omitempty"`
}

// ScrapeData holds the various pieces of data that the scrape operation
// may return. Only a subset of fields are modelled here; additional
// fields can be added as needed by consumers.
type ScrapeData struct {
	Markdown   string   `json:"markdown,omitempty"`
	Summary    string   `json:"summary,omitempty"`
	HTML       string   `json:"html,omitempty"`
	RawHTML    string   `json:"rawHtml,omitempty"`
	Screenshot string   `json:"screenshot,omitempty"`
	Links      []string `json:"links,omitempty"`
}
