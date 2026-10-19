package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// SearchActionInfo is what runs a search action hit.
type SearchActionInfo struct {
	ID      string `json:"id"`
	Ability string `json:"ability"`
	Confirm bool   `json:"confirm"`
	// MCPTool is the exact MCP tool to call, MCPArgs its filled-in arguments
	// and MCPNeeds the inputs the caller must still supply.
	MCPTool  string            `json:"mcp_tool,omitempty"`
	MCPArgs  map[string]string `json:"mcp_args,omitempty"`
	MCPNeeds []string          `json:"mcp_needs,omitempty"`
}

// SearchSpan is a half-open range of code points in a hit's title.
type SearchSpan struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// SearchHit is one unified search result.
type SearchHit struct {
	Ref        string            `json:"ref"`
	Kind       string            `json:"kind"`
	Title      string            `json:"title"`
	Subtitle   string            `json:"subtitle,omitempty"`
	AppName    string            `json:"app_name,omitempty"`
	Env        string            `json:"env,omitempty"`
	NodeName   string            `json:"node_name,omitempty"`
	Tags       []string          `json:"tags,omitempty"`
	Href       string            `json:"href,omitempty"`
	TitleSpans []SearchSpan      `json:"title_spans,omitempty"`
	Score      float64           `json:"score"`
	Action     *SearchActionInfo `json:"action,omitempty"`
}

// SearchResponse is a page of hits.
type SearchResponse struct {
	Hits       []SearchHit `json:"hits"`
	NextCursor string      `json:"next_cursor,omitempty"`
	Total      int         `json:"total"`
}

// SavedSearch is a stored query.
type SavedSearch struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Query     string    `json:"query"`
	CreatedAt time.Time `json:"created_at"`
}

// Search calls GET /api/v1/search.
func (c *Client) Search(ctx context.Context, q, kind string, limit int, cursor string) (SearchResponse, error) {
	v := url.Values{"q": {q}}
	if kind != "" {
		v.Set("kind", kind)
	}
	if limit > 0 {
		v.Set("limit", strconv.Itoa(limit))
	}
	if cursor != "" {
		v.Set("cursor", cursor)
	}
	var out SearchResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/search?"+v.Encode(), nil, &out)
	return out, err
}

// ListSavedSearches calls GET /api/v1/search/saved.
func (c *Client) ListSavedSearches(ctx context.Context) ([]SavedSearch, error) {
	var out struct {
		Saved []SavedSearch `json:"saved"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/search/saved", nil, &out)
	return out.Saved, err
}

// SaveSearch calls POST /api/v1/search/saved.
func (c *Client) SaveSearch(ctx context.Context, name, query string) (SavedSearch, error) {
	var out SavedSearch
	err := c.do(ctx, http.MethodPost, "/api/v1/search/saved", map[string]string{"name": name, "query": query}, &out)
	return out, err
}

// DeleteSavedSearch calls DELETE /api/v1/search/saved/{id}.
func (c *Client) DeleteSavedSearch(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/search/saved/"+url.PathEscape(id), nil, nil)
}
