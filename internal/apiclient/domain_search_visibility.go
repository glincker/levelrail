package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// DomainSearchVisibilityResource is the state of one domain's search engine visibility.
type DomainSearchVisibilityResource struct {
	Domain string `json:"domain"`
	Hidden bool   `json:"hidden"`
}

func domainSearchVisibilityPath(name, domain string) string {
	return "/api/v1/apps/" + url.PathEscape(name) + "/domains/" + url.PathEscape(domain) + "/search-visibility"
}

// GetDomainSearchVisibility calls GET .../domains/{domain}/search-visibility.
func (c *Client) GetDomainSearchVisibility(ctx context.Context, name, domain string) (DomainSearchVisibilityResource, error) {
	var out DomainSearchVisibilityResource
	err := c.do(ctx, http.MethodGet, domainSearchVisibilityPath(name, domain), nil, &out)
	return out, err
}

// SetDomainSearchVisibility calls PUT .../domains/{domain}/search-visibility.
func (c *Client) SetDomainSearchVisibility(ctx context.Context, name, domain string, hidden bool) (DomainSearchVisibilityResource, error) {
	var out DomainSearchVisibilityResource
	body := struct {
		Hidden bool `json:"hidden"`
	}{hidden}
	err := c.do(ctx, http.MethodPut, domainSearchVisibilityPath(name, domain), body, &out)
	return out, err
}
