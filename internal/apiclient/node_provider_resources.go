package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// NodeProviderServer is one machine in a provider account.
type NodeProviderServer struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Region             string `json:"region"`
	Size               string `json:"size"`
	Status             string `json:"status"`
	PublicIPv4         string `json:"public_ipv4"`
	ManagedByLevelrail bool   `json:"managed_by_levelrail"`
	EnrolledNode       *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"enrolled_node"`
}

// ListNodeProviderServers calls GET /api/v1/node-providers/{provider}/resources.
// A non-empty region asks the provider to list that region (optional).
func (c *Client) ListNodeProviderServers(ctx context.Context, provider, region string, refresh bool) ([]NodeProviderServer, error) {
	path := nodeProvidersPath() + "/" + PathEscape(provider) + "/resources"
	q := url.Values{}
	if refresh {
		q.Set("refresh", "1")
	}
	if region != "" {
		q.Set("region", region)
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out struct {
		Resources []NodeProviderServer `json:"resources"`
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out.Resources, err
}
