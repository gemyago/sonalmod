package firecrawl

// ScrapeRequest represents the payload for a /scrape request.
// It composes the common ScrapeOptions and adds the required URL field
// as well as the zeroDataRetention flag.
type ScrapeRequest struct {
	ScrapeOptions `json:",inline"` // embedded first per lint

	URL               string `json:"url"`
	ZeroDataRetention bool   `json:"zeroDataRetention,omitempty"`
}
