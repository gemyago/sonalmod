package firecrawl

type ScrapeOptions struct {
	Formats             []Format          `json:"formats,omitempty"`
	OnlyMainContent     bool              `json:"onlyMainContent,omitempty"`
	IncludeTags         []string          `json:"includeTags,omitempty"`
	ExcludeTags         []string          `json:"excludeTags,omitempty"`
	MaxAge              int               `json:"maxAge,omitempty"`
	Headers             map[string]string `json:"headers,omitempty"`
	WaitFor             int               `json:"waitFor,omitempty"`
	Mobile              bool              `json:"mobile,omitempty"`
	SkipTLSVerification bool              `json:"skipTlsVerification,omitempty"`
	Timeout             int               `json:"timeout,omitempty"`
	Location            *Location         `json:"location,omitempty"`
	RemoveBase64Images  bool              `json:"removeBase64Images,omitempty"`
	BlockAds            bool              `json:"blockAds,omitempty"`
	Proxy               string            `json:"proxy,omitempty"`
	StoreInCache        bool              `json:"storeInCache,omitempty"`
}

type Location struct {
	Country   string   `json:"country,omitempty"`
	Languages []string `json:"languages,omitempty"`
}
