package firecrawl

import (
	"github.com/gemyago/sonalmod/runtime/agent"
	ifc "github.com/gemyago/sonalmod/tools/firecrawl/internal/firecrawl"
)

func firecrawlAgentTools(client *ifc.Client) []agent.DefinedTool {
	scrape := agent.NewToolDef[ifc.ScrapeRequest, *ifc.ScrapeResponse](
		"firecrawl_scrape",
		"Scrape a single URL with Firecrawl and return markdown, HTML, and related content.",
		func(tc *agent.ToolContext, in ifc.ScrapeRequest) (*ifc.ScrapeResponse, error) {
			return client.Scrape(tc, ifc.ScrapeParams{Request: &in})
		},
	)
	crawl := agent.NewToolDef[ifc.CrawlRequest, *ifc.CrawlResponse](
		"firecrawl_crawl",
		"Start a Firecrawl crawl job for a URL and return the crawl job id.",
		func(tc *agent.ToolContext, in ifc.CrawlRequest) (*ifc.CrawlResponse, error) {
			return client.Crawl(tc, ifc.CrawlParams{Request: &in})
		},
	)
	return []agent.DefinedTool{scrape, crawl}
}
