package apiclient

import (
	"context"
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
	return getJSON[[]DomainWatch](ctx, c, "/api/v1/domains/watch")
}

// GetDomainWatch calls GET /api/v1/domains/{domain}/watch.
func (c *Client) GetDomainWatch(ctx context.Context, domain string) (DomainWatch, error) {
	return getJSON[DomainWatch](ctx, c, "/api/v1/domains/"+PathEscape(domain)+"/watch")
}
