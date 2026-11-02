package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// ParkedDomain mirrors internal/api's parkedDomainResource.
type ParkedDomain struct {
	Domain     string `json:"domain"`
	Kind       string `json:"kind"`
	ParkedFrom string `json:"parked_from,omitempty"`
	// ParkedFromKind is app or static_site: what ParkedFrom names.
	ParkedFromKind string `json:"parked_from_kind,omitempty"`
	// ParkedFromEnvironment is the environment the domain left, empty for the default list.
	ParkedFromEnvironment string `json:"parked_from_environment,omitempty"`
	Note                  string `json:"note,omitempty"`
	ParkedBy              string `json:"parked_by,omitempty"`
	ParkedAt              string `json:"parked_at,omitempty"`
	// HasSettings is true when attach can restore the settings kept by park.
	HasSettings bool `json:"has_settings,omitempty"`
	// DNS and TLS are the live state computed when the list is read.
	DNS *ParkedDNS `json:"dns,omitempty"`
	TLS *ParkedTLS `json:"tls,omitempty"`
}

// ParkedDNS is whether a parked domain still points at this server.
type ParkedDNS struct {
	Status    string `json:"status"`
	CheckedAt string `json:"checked_at,omitempty"`
}

// ParkedTLS is a parked domain's certificate state; DaysLeft is negative once expired.
type ParkedTLS struct {
	State    string `json:"state"`
	NotAfter string `json:"not_after,omitempty"`
	DaysLeft *int   `json:"days_left,omitempty"`
}

// DomainLifecycleRequest is the body of park, move and attach.
type DomainLifecycleRequest struct {
	Domain string `json:"domain,omitempty"`
	App    string `json:"app,omitempty"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Note   string `json:"note,omitempty"`
	// Environment picks the environment domain set for park and attach.
	Environment string `json:"environment,omitempty"`
	// FromEnvironment and ToEnvironment pick the sets of a move.
	FromEnvironment string `json:"from_environment,omitempty"`
	ToEnvironment   string `json:"to_environment,omitempty"`
	// TargetKind is app (default) or static_site for park and attach; the name is in App.
	TargetKind string `json:"target_kind,omitempty"`
	// FromKind and ToKind are app (default) or static_site for the names of a move.
	FromKind string `json:"from_kind,omitempty"`
	ToKind   string `json:"to_kind,omitempty"`
	Confirm  bool   `json:"confirm,omitempty"`
	// DiscardSettings makes attach drop the settings kept by park.
	DiscardSettings bool `json:"discard_settings,omitempty"`
}

// DomainLifecycleResult mirrors internal/api's domainLifecycleResponse.
type DomainLifecycleResult struct {
	Domain string `json:"domain"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	// Kind, FromKind and ToKind echo whether each name is an app or a static_site.
	Kind     string `json:"kind,omitempty"`
	FromKind string `json:"from_kind,omitempty"`
	ToKind   string `json:"to_kind,omitempty"`
	// Message carries a note such as settings that were not restored.
	Message string `json:"message,omitempty"`
	// Environment, FromEnvironment and ToEnvironment echo the resolved environment names.
	Environment     string `json:"environment,omitempty"`
	FromEnvironment string `json:"from_environment,omitempty"`
	ToEnvironment   string `json:"to_environment,omitempty"`
	Applied         bool   `json:"applied"`
	// RestoredSettings names the settings kinds an attach restored.
	RestoredSettings []string `json:"restored_settings,omitempty"`
}

// ListParkedDomains calls GET /api/v1/domains/parked.
func (c *Client) ListParkedDomains(ctx context.Context) ([]ParkedDomain, error) {
	var out []ParkedDomain
	err := c.do(ctx, http.MethodGet, "/api/v1/domains/parked", nil, &out)
	return out, err
}

// ParkDomain calls POST /api/v1/domains/park.
func (c *Client) ParkDomain(ctx context.Context, req DomainLifecycleRequest) (DomainLifecycleResult, error) {
	var out DomainLifecycleResult
	err := c.do(ctx, http.MethodPost, "/api/v1/domains/park", req, &out)
	return out, err
}

// MoveDomain calls POST /api/v1/domains/move.
func (c *Client) MoveDomain(ctx context.Context, req DomainLifecycleRequest) (DomainLifecycleResult, error) {
	var out DomainLifecycleResult
	err := c.do(ctx, http.MethodPost, "/api/v1/domains/move", req, &out)
	return out, err
}

// AttachParkedDomain calls POST /api/v1/domains/parked/{domain}/attach.
func (c *Client) AttachParkedDomain(ctx context.Context, domain, app, environment string, discardSettings bool) (DomainLifecycleResult, error) {
	return c.AttachParkedDomainTo(ctx, domain, DomainLifecycleRequest{App: app, Environment: environment, DiscardSettings: discardSettings})
}

// AttachParkedDomainTo is AttachParkedDomain with the full request, so a
// static site target can be named through TargetKind.
func (c *Client) AttachParkedDomainTo(ctx context.Context, domain string, req DomainLifecycleRequest) (DomainLifecycleResult, error) {
	var out DomainLifecycleResult
	err := c.do(ctx, http.MethodPost, "/api/v1/domains/parked/"+url.PathEscape(domain)+"/attach", req, &out)
	return out, err
}

// DeleteParkedDomain calls DELETE /api/v1/domains/parked/{domain}.
func (c *Client) DeleteParkedDomain(ctx context.Context, domain string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/domains/parked/"+url.PathEscape(domain), nil, nil)
}

// DomainBatchItem is one domain of a bulk request; the fields used depend on the action.
type DomainBatchItem struct {
	Domain          string `json:"domain"`
	App             string `json:"app,omitempty"`
	From            string `json:"from,omitempty"`
	To              string `json:"to,omitempty"`
	Note            string `json:"note,omitempty"`
	Environment     string `json:"environment,omitempty"`
	TargetKind      string `json:"target_kind,omitempty"`
	FromKind        string `json:"from_kind,omitempty"`
	ToKind          string `json:"to_kind,omitempty"`
	FromEnvironment string `json:"from_environment,omitempty"`
	ToEnvironment   string `json:"to_environment,omitempty"`
}

// DomainBatchRequest is the body of POST /api/v1/domains/batch; Action is
// park, move, attach or delete_parked.
type DomainBatchRequest struct {
	Action          string            `json:"action"`
	Items           []DomainBatchItem `json:"items"`
	Confirm         bool              `json:"confirm,omitempty"`
	DiscardSettings bool              `json:"discard_settings,omitempty"`
}

// DomainBatchResult is the outcome of one item, with the HTTP status the single route would give.
type DomainBatchResult struct {
	Domain               string   `json:"domain"`
	OK                   bool     `json:"ok"`
	Status               int      `json:"status"`
	Error                string   `json:"error,omitempty"`
	Code                 string   `json:"code,omitempty"`
	RequiresConfirmation bool     `json:"requires_confirmation,omitempty"`
	Message              string   `json:"message,omitempty"`
	RestoredSettings     []string `json:"restored_settings,omitempty"`
}

// DomainBatchResponse mirrors internal/api's domainBatchResponse.
type DomainBatchResponse struct {
	BatchID string              `json:"batch_id"`
	Action  string              `json:"action"`
	Results []DomainBatchResult `json:"results"`
	Summary struct {
		Total                int `json:"total"`
		Succeeded            int `json:"succeeded"`
		Failed               int `json:"failed"`
		RequiresConfirmation int `json:"requires_confirmation"`
	} `json:"summary"`
}

// DomainsBatch calls POST /api/v1/domains/batch.
func (c *Client) DomainsBatch(ctx context.Context, req DomainBatchRequest) (DomainBatchResponse, error) {
	var out DomainBatchResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/domains/batch", req, &out)
	return out, err
}
