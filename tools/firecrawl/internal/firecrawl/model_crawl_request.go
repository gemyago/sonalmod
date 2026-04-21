package firecrawl

type CrawlRequest struct {
	URL                   string         `json:"url"`
	Prompt                string         `json:"prompt,omitempty"`
	ExcludePaths          []string       `json:"excludePaths,omitempty"`
	IncludePaths          []string       `json:"includePaths,omitempty"`
	MaxDiscoveryDepth     int            `json:"maxDiscoveryDepth,omitempty"`
	Sitemap               string         `json:"sitemap,omitempty"`
	IgnoreQueryParameters bool           `json:"ignoreQueryParameters,omitempty"`
	RegexOnFullURL        bool           `json:"regexOnFullURL,omitempty"`
	Limit                 int            `json:"limit,omitempty"`
	CrawlEntireDomain     bool           `json:"crawlEntireDomain,omitempty"`
	AllowExternalLinks    bool           `json:"allowExternalLinks,omitempty"`
	AllowSubdomains       bool           `json:"allowSubdomains,omitempty"`
	Delay                 float64        `json:"delay,omitempty"`
	MaxConcurrency        int            `json:"maxConcurrency,omitempty"`
	Webhook               *Webhook       `json:"webhook,omitempty"`
	ScrapeOptions         *ScrapeOptions `json:"scrapeOptions,omitempty"`
	ZeroDataRetention     bool           `json:"zeroDataRetention,omitempty"`
}
