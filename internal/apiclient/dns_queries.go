package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// DNSQueryRecord is one mesh DNS query the control plane logged.
type DNSQueryRecord struct {
	Time          time.Time `json:"time"`
	Source        string    `json:"source,omitempty"`
	App           string    `json:"app,omitempty"`
	QName         string    `json:"qname"`
	QType         string    `json:"qtype"`
	RCode         string    `json:"rcode"`
	Answers       int       `json:"answers"`
	LatencyMicros int64     `json:"latency_us"`
	Reason        string    `json:"reason"`
}

// DNSQueriesResult is the body of GET /api/v1/dns/queries.
type DNSQueriesResult struct {
	Enabled bool             `json:"enabled"`
	Privacy string           `json:"privacy"`
	Queries []DNSQueryRecord `json:"queries"`
}

// DNSAppCounters are per-app mesh DNS totals.
type DNSAppCounters struct {
	App      string `json:"app"`
	Queries  uint64 `json:"queries"`
	NXDomain uint64 `json:"nxdomain"`
	Refused  uint64 `json:"refused"`
}

// DNSStatsResult is the body of GET /api/v1/dns/stats.
type DNSStatsResult struct {
	Enabled bool             `json:"enabled"`
	Privacy string           `json:"privacy"`
	Apps    []DNSAppCounters `json:"apps"`
	Total   DNSAppCounters   `json:"total"`
}

// DNSQueries calls GET /api/v1/dns/queries with the given filters.
func (c *Client) DNSQueries(ctx context.Context, q url.Values) (DNSQueriesResult, error) {
	var out DNSQueriesResult
	path := "/api/v1/dns/queries"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// DNSStats calls GET /api/v1/dns/stats.
func (c *Client) DNSStats(ctx context.Context) (DNSStatsResult, error) {
	return getJSON[DNSStatsResult](ctx, c, "/api/v1/dns/stats")
}
