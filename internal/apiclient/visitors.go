package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// VisitorItem is one ranked label in a visitor summary.
type VisitorItem struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// VisitorBucket is one sparkline step.
type VisitorBucket struct {
	Start    time.Time `json:"start"`
	Requests int       `json:"requests"`
	Visitors int       `json:"visitors"`
}

// VisitorDomain is the per-domain split.
type VisitorDomain struct {
	Domain   string `json:"domain"`
	Requests int    `json:"requests"`
	Visitors int    `json:"visitors"`
}

// Visitors mirrors the body of GET /api/v1/apps/{name}/visitors.
type Visitors struct {
	App         string          `json:"app"`
	Domain      string          `json:"domain,omitempty"`
	Window      string          `json:"window"`
	Since       time.Time       `json:"since"`
	GeneratedAt time.Time       `json:"generated_at"`
	Requests    int             `json:"requests"`
	Visitors    int             `json:"visitors"`
	Status      map[string]int  `json:"status"`
	TopPaths    []VisitorItem   `json:"top_paths"`
	TopRefs     []VisitorItem   `json:"top_referrers"`
	Series      []VisitorBucket `json:"series"`
	Domains     []VisitorDomain `json:"domains"`
	OldestKept  time.Time       `json:"oldest_record"`
	SaltPeriod  string          `json:"salt_period"`
}

// GetVisitors calls GET /api/v1/apps/{name}/visitors.
func (c *Client) GetVisitors(ctx context.Context, app, domain, window string) (Visitors, error) {
	q := url.Values{}
	if domain != "" {
		q.Set("domain", domain)
	}
	if window != "" {
		q.Set("window", window)
	}
	path := "/api/v1/apps/" + PathEscape(app) + "/visitors"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out Visitors
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// ExportVisitorsCSV calls GET /api/v1/apps/{name}/visitors/export and returns the CSV bytes.
func (c *Client) ExportVisitorsCSV(ctx context.Context, app, domain, window string) ([]byte, error) {
	q := url.Values{"format": {"csv"}}
	if domain != "" {
		q.Set("domain", domain)
	}
	if window != "" {
		q.Set("window", window)
	}
	return c.downloadRaw(ctx, "/api/v1/apps/"+PathEscape(app)+"/visitors/export?"+q.Encode())
}
