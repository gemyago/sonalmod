package firecrawl

import (
	"context"
	"fmt"
)

type CrawlParams struct {
	Request *CrawlRequest
}

func (c *Client) Crawl(ctx context.Context, params CrawlParams) (*CrawlResponse, error) {
	var response CrawlResponse
	err := sendRequest(ctx, c.httpClient, sendRequestParams[CrawlRequest, CrawlResponse]{
		Method: "POST",
		URL:    c.baseURL + "/crawl",
		Body:   params.Request,
		Target: &response,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start crawl: %w", err)
	}

	return &response, nil
}
