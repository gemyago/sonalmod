package firecrawl

import (
	"context"
	"fmt"
)

// ScrapeParams contains parameters for performing a scrape request.
type ScrapeParams struct {
	Request *ScrapeRequest
}

// Scrape sends a POST /scrape request to the Firecrawl service.
func (c *Client) Scrape(ctx context.Context, params ScrapeParams) (*ScrapeResponse, error) {
	var response ScrapeResponse
	err := sendRequest(ctx, c.httpClient, sendRequestParams[ScrapeRequest, ScrapeResponse]{
		Method: "POST",
		URL:    c.baseURL + "/scrape",
		Body:   params.Request,
		Target: &response,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scrape url: %w", err)
	}

	return &response, nil
}
