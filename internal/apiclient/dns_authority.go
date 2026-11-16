package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// DNSAuthorityNameServer is one name server of a domain and the provider it maps to.
type DNSAuthorityNameServer struct {
	Name     string   `json:"name"`
	Provider string   `json:"provider"`
	IPs      []string `json:"ips,omitempty"`
}

// DNSAuthority is the response of GET /api/v1/dns/authority.
type DNSAuthority struct {
	Domain      string                   `json:"domain"`
	NameServers []DNSAuthorityNameServer `json:"name_servers"`
	Providers   []string                 `json:"providers"`
	Answers     []string                 `json:"answers,omitempty"`
	CNAME       string                   `json:"cname,omitempty"`
	Proxied     bool                     `json:"proxied"`
	PointsHere  bool                     `json:"points_here"`
	TTL         *time.Duration           `json:"ttl,omitempty"`
	Risks       []string                 `json:"risks"`
}

// DNSAuthority calls GET /api/v1/dns/authority.
func (c *Client) DNSAuthority(ctx context.Context, domain string) (DNSAuthority, error) {
	var out DNSAuthority
	err := c.do(ctx, http.MethodGet, "/api/v1/dns/authority?"+url.Values{"domain": {domain}}.Encode(), nil, &out)
	return out, err
}
