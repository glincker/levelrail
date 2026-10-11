package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// DomainDNSResult mirrors internal/api's domainDNSResult.
type DomainDNSResult struct {
	Domain   string              `json:"domain"`
	DNS      string              `json:"dns"`
	Planned  string              `json:"planned,omitempty"`
	Provider string              `json:"provider,omitempty"`
	Zone     string              `json:"zone,omitempty"`
	Record   *DNSRecordResource  `json:"record,omitempty"`
	Replaced []DNSRecordResource `json:"replaced,omitempty"`
	Proxied  bool                `json:"proxied,omitempty"`
	Message  string              `json:"message,omitempty"`
}

// DomainAutomationPolicy mirrors internal/api's domainAutomationPolicy.
type DomainAutomationPolicy struct {
	AutoDNS               bool   `json:"auto_dns"`
	AutoProxyRoute        bool   `json:"auto_proxy_route"`
	ForceHTTPS            bool   `json:"force_https"`
	WWWPolicy             string `json:"www_policy"`
	AttachWWWCounterpart  bool   `json:"attach_www_counterpart"`
	WildcardForBaseDomain bool   `json:"wildcard_for_base_domain"`
	VerifyAfter           bool   `json:"verify_after"`
}

// DomainAutomationOverride sets only the keys given.
type DomainAutomationOverride struct {
	AutoDNS               *bool   `json:"auto_dns,omitempty"`
	AutoProxyRoute        *bool   `json:"auto_proxy_route,omitempty"`
	ForceHTTPS            *bool   `json:"force_https,omitempty"`
	WWWPolicy             *string `json:"www_policy,omitempty"`
	AttachWWWCounterpart  *bool   `json:"attach_www_counterpart,omitempty"`
	WildcardForBaseDomain *bool   `json:"wildcard_for_base_domain,omitempty"`
	VerifyAfter           *bool   `json:"verify_after,omitempty"`
}

// DomainAutomationResource mirrors GET/PUT /api/v1/settings/domain-automation.
type DomainAutomationResource struct {
	DomainAutomationPolicy
	DNSProvider      string           `json:"dns_provider"`
	ProxyIntegration bool             `json:"proxy_integration_enabled"`
	WildcardDNS      *DomainDNSResult `json:"wildcard_dns,omitempty"`
}

// GoLiveStep mirrors internal/api's goLiveStep.
type GoLiveStep struct {
	ID         string             `json:"id"`
	State      string             `json:"state"`
	Detail     string             `json:"detail,omitempty"`
	Provider   string             `json:"provider,omitempty"`
	Record     *DNSRecordResource `json:"record,omitempty"`
	Resolvers  []ResolverResult   `json:"resolvers,omitempty"`
	Issuer     string             `json:"issuer,omitempty"`
	NotAfter   *time.Time         `json:"not_after,omitempty"`
	URL        string             `json:"url,omitempty"`
	HTTPStatus int                `json:"http_status,omitempty"`
}

// ResolverResult is what one resolver answered for a domain.
type ResolverResult struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// GoLiveResult mirrors internal/api's goLiveResult.
type GoLiveResult struct {
	RunID     string                 `json:"run_id,omitempty"`
	App       string                 `json:"app"`
	Domain    string                 `json:"domain"`
	State     string                 `json:"state"`
	URL       string                 `json:"url,omitempty"`
	Steps     []GoLiveStep           `json:"steps"`
	DNS       []DomainDNSResult      `json:"dns,omitempty"`
	Policy    DomainAutomationPolicy `json:"policy"`
	Undoable  bool                   `json:"undoable,omitempty"`
	CheckedAt string                 `json:"checked_at"`
}

// GoLiveRequest is the body of POST .../go-live.
type GoLiveRequest struct {
	Automation *DomainAutomationOverride `json:"automation,omitempty"`
	DNS        string                    `json:"dns,omitempty"`
	Replace    bool                      `json:"replace,omitempty"`
}

// GoLivePlanRequest is the body of POST .../domains/go-live/plan.
type GoLivePlanRequest struct {
	Domain     string                    `json:"domain,omitempty"`
	Domains    []string                  `json:"domains,omitempty"`
	Automation *DomainAutomationOverride `json:"automation,omitempty"`
}

// GoLivePlanResponse is the dry run result.
type GoLivePlanResponse struct {
	Plans []GoLiveResult `json:"plans"`
}

// AutomationRun is one entry of the automation history.
type AutomationRun struct {
	ID        string       `json:"id"`
	App       string       `json:"app"`
	Domain    string       `json:"domain"`
	Result    string       `json:"result"`
	Steps     []GoLiveStep `json:"steps"`
	CreatedAt time.Time    `json:"created_at"`
	UndoneAt  *time.Time   `json:"undone_at,omitempty"`
	Undoable  bool         `json:"undoable"`
}

// UndoResult is the response of an undo.
type UndoResult struct {
	AutomationRun
	Reverted []string `json:"reverted"`
	Skipped  []string `json:"skipped,omitempty"`
}

// DNSZoneResult mirrors GET /api/v1/dns/zone.
type DNSZoneResult struct {
	Domain     string `json:"domain"`
	Provider   string `json:"provider"`
	Configured bool   `json:"configured"`
	Found      bool   `json:"found"`
	Zone       string `json:"zone,omitempty"`
	Message    string `json:"message,omitempty"`
}

// BaseDomainBackfillItem is one app of a backfill.
type BaseDomainBackfillItem struct {
	App    string           `json:"app"`
	Domain string           `json:"domain"`
	Status string           `json:"status"`
	DNS    *DomainDNSResult `json:"dns,omitempty"`
}

// BaseDomainBackfill is the backfill response.
type BaseDomainBackfill struct {
	DryRun     bool                     `json:"dry_run"`
	BaseDomain string                   `json:"base_domain"`
	Items      []BaseDomainBackfillItem `json:"items"`
}

func goLivePath(app, domain string) string {
	return "/api/v1/apps/" + PathEscape(app) + "/domains/" + PathEscape(domain) + "/go-live"
}

// GetGoLive calls GET .../domains/{domain}/go-live (a poll friendly status).
func (c *Client) GetGoLive(ctx context.Context, app, domain string) (GoLiveResult, error) {
	var out GoLiveResult
	err := c.do(ctx, http.MethodGet, goLivePath(app, domain), nil, &out)
	return out, err
}

// RunGoLive calls POST .../domains/{domain}/go-live.
func (c *Client) RunGoLive(ctx context.Context, app, domain string, req GoLiveRequest) (GoLiveResult, error) {
	var out GoLiveResult
	err := c.do(ctx, http.MethodPost, goLivePath(app, domain), req, &out)
	return out, err
}

// PlanGoLive calls POST .../domains/go-live/plan.
func (c *Client) PlanGoLive(ctx context.Context, app string, req GoLivePlanRequest) (GoLivePlanResponse, error) {
	var out GoLivePlanResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(app)+"/domains/go-live/plan", req, &out)
	return out, err
}

// GetDomainAutomation calls GET /api/v1/settings/domain-automation.
func (c *Client) GetDomainAutomation(ctx context.Context) (DomainAutomationResource, error) {
	var out DomainAutomationResource
	err := c.do(ctx, http.MethodGet, "/api/v1/settings/domain-automation", nil, &out)
	return out, err
}

// UpdateDomainAutomation calls PUT /api/v1/settings/domain-automation.
func (c *Client) UpdateDomainAutomation(ctx context.Context, p DomainAutomationOverride) (DomainAutomationResource, error) {
	var out DomainAutomationResource
	err := c.do(ctx, http.MethodPut, "/api/v1/settings/domain-automation", p, &out)
	return out, err
}

// ListAutomationRuns calls GET /api/v1/domains/automation/runs.
func (c *Client) ListAutomationRuns(ctx context.Context, limit int) ([]AutomationRun, error) {
	path := "/api/v1/domains/automation/runs"
	if limit > 0 {
		path += "?" + url.Values{"limit": {strconv.Itoa(limit)}}.Encode()
	}
	var out []AutomationRun
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// UndoAutomationRun calls POST /api/v1/domains/automation/runs/{id}/undo.
func (c *Client) UndoAutomationRun(ctx context.Context, id string) (UndoResult, error) {
	var out UndoResult
	err := c.do(ctx, http.MethodPost, "/api/v1/domains/automation/runs/"+PathEscape(id)+"/undo", nil, &out)
	return out, err
}

// DNSZone calls GET /api/v1/dns/zone?domain=.
func (c *Client) DNSZone(ctx context.Context, domain string) (DNSZoneResult, error) {
	var out DNSZoneResult
	err := c.do(ctx, http.MethodGet, "/api/v1/dns/zone?"+url.Values{"domain": {domain}}.Encode(), nil, &out)
	return out, err
}

// BackfillBaseDomain calls POST /api/v1/settings/ingress/apps-base-domain/backfill.
func (c *Client) BackfillBaseDomain(ctx context.Context, confirm bool) (BaseDomainBackfill, error) {
	var out BaseDomainBackfill
	err := c.do(ctx, http.MethodPost, "/api/v1/settings/ingress/apps-base-domain/backfill", map[string]bool{"confirm": confirm}, &out)
	return out, err
}
