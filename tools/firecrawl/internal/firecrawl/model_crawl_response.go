package firecrawl

type CrawlResponse struct {
	Success bool   `json:"success"`
	ID      string `json:"id"`
	URL     string `json:"url"`
}
