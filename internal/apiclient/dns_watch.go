package apiclient

import (
	"context"
	"net/http"
	"time"
)

// DomainWatch is one domain the control plane's DNS watcher follows.
type DomainWatch struct {
	Domain      string    `json:"domain"`
	App         string    `json:"app"`
	State       string    `json:"state"`
	Since       time.Time `json:"since"`
	LastChecked time.Time `json:"last_checked"`
	NextCheck   time.Time `json:"next_check"`
	Attempts    int       `json:"attempts"`
	StalledAt   time.Time `json:"stalled_at"`
	Stopped     bool      `json:"stopped"`
	NotifiedAt  time.Time `json:"notified_at"`
}

// ListDomainWatches calls GET /api/v1/domains/watch.
func (c *Client) ListDomainWatches(ctx context.Context) ([]DomainWatch, error) {
	var out []DomainWatch
	err := c.do(ctx, http.MethodGet, "/api/v1/domains/watch", nil, &out)
	return out, err
}

// GetDomainWatch calls GET /api/v1/domains/{domain}/watch.
func (c *Client) GetDomainWatch(ctx context.Context, domain string) (DomainWatch, error) {
	var out DomainWatch
	err := c.do(ctx, http.MethodGet, "/api/v1/domains/"+PathEscape(domain)+"/watch", nil, &out)
	return out, err
}
