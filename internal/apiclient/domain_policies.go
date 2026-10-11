package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func domainPolicyPath(name, domain, suffix string) string {
	return "/api/v1/apps/" + url.PathEscape(name) + "/domains/" + url.PathEscape(domain) + "/" + suffix
}

// HeadersPolicyResource is GET/PUT .../domains/{domain}/headers.
type HeadersPolicyResource struct {
	Domain     string                `json:"domain"`
	Kind       string                `json:"kind"`
	Configured bool                  `json:"configured"`
	Spec       trafficpolicy.Headers `json:"spec"`
	UpdatedAt  string                `json:"updated_at,omitempty"`
}

// ForwardersPolicyResource is GET/PUT .../domains/{domain}/forwarders.
type ForwardersPolicyResource struct {
	Domain     string                   `json:"domain"`
	Kind       string                   `json:"kind"`
	Configured bool                     `json:"configured"`
	Spec       trafficpolicy.Forwarders `json:"spec"`
	UpdatedAt  string                   `json:"updated_at,omitempty"`
}

// GeoPolicyResource is GET/PUT .../domains/{domain}/geo. Spec is nil when
// no geo rule is configured.
type GeoPolicyResource struct {
	Domain     string             `json:"domain"`
	Kind       string             `json:"kind"`
	Configured bool               `json:"configured"`
	Spec       *trafficpolicy.Geo `json:"spec"`
	UpdatedAt  string             `json:"updated_at,omitempty"`
}

// CachePolicyResource is GET/PUT .../domains/{domain}/cache.
type CachePolicyResource struct {
	Domain     string              `json:"domain"`
	Kind       string              `json:"kind"`
	Configured bool                `json:"configured"`
	Spec       trafficpolicy.Cache `json:"spec"`
	UpdatedAt  string              `json:"updated_at,omitempty"`
}

// GetDomainHeaders calls GET .../domains/{domain}/headers.
func (c *Client) GetDomainHeaders(ctx context.Context, name, domain string) (HeadersPolicyResource, error) {
	var out HeadersPolicyResource
	err := c.do(ctx, http.MethodGet, domainPolicyPath(name, domain, trafficpolicy.KindHeaders), nil, &out)
	return out, err
}

// SetDomainHeaders calls PUT .../domains/{domain}/headers.
func (c *Client) SetDomainHeaders(ctx context.Context, name, domain string, spec trafficpolicy.Headers) (HeadersPolicyResource, error) {
	var out HeadersPolicyResource
	err := c.do(ctx, http.MethodPut, domainPolicyPath(name, domain, trafficpolicy.KindHeaders), spec, &out)
	return out, err
}

// ClearDomainHeaders calls DELETE .../domains/{domain}/headers.
func (c *Client) ClearDomainHeaders(ctx context.Context, name, domain string) (HeadersPolicyResource, error) {
	var out HeadersPolicyResource
	err := c.do(ctx, http.MethodDelete, domainPolicyPath(name, domain, trafficpolicy.KindHeaders), nil, &out)
	return out, err
}

// GetDomainForwarders calls GET .../domains/{domain}/forwarders.
func (c *Client) GetDomainForwarders(ctx context.Context, name, domain string) (ForwardersPolicyResource, error) {
	var out ForwardersPolicyResource
	err := c.do(ctx, http.MethodGet, domainPolicyPath(name, domain, trafficpolicy.KindForwarders), nil, &out)
	return out, err
}

// SetDomainForwarders calls PUT .../domains/{domain}/forwarders.
func (c *Client) SetDomainForwarders(ctx context.Context, name, domain string, spec trafficpolicy.Forwarders) (ForwardersPolicyResource, error) {
	var out ForwardersPolicyResource
	err := c.do(ctx, http.MethodPut, domainPolicyPath(name, domain, trafficpolicy.KindForwarders), spec, &out)
	return out, err
}

// ClearDomainForwarders calls DELETE .../domains/{domain}/forwarders.
func (c *Client) ClearDomainForwarders(ctx context.Context, name, domain string) (ForwardersPolicyResource, error) {
	var out ForwardersPolicyResource
	err := c.do(ctx, http.MethodDelete, domainPolicyPath(name, domain, trafficpolicy.KindForwarders), nil, &out)
	return out, err
}

// GetDomainGeo calls GET .../domains/{domain}/geo.
func (c *Client) GetDomainGeo(ctx context.Context, name, domain string) (GeoPolicyResource, error) {
	var out GeoPolicyResource
	err := c.do(ctx, http.MethodGet, domainPolicyPath(name, domain, trafficpolicy.KindGeo), nil, &out)
	return out, err
}

// SetDomainGeo calls PUT .../domains/{domain}/geo.
func (c *Client) SetDomainGeo(ctx context.Context, name, domain string, spec trafficpolicy.Geo) (GeoPolicyResource, error) {
	var out GeoPolicyResource
	err := c.do(ctx, http.MethodPut, domainPolicyPath(name, domain, trafficpolicy.KindGeo), spec, &out)
	return out, err
}

// ClearDomainGeo calls DELETE .../domains/{domain}/geo.
func (c *Client) ClearDomainGeo(ctx context.Context, name, domain string) (GeoPolicyResource, error) {
	var out GeoPolicyResource
	err := c.do(ctx, http.MethodDelete, domainPolicyPath(name, domain, trafficpolicy.KindGeo), nil, &out)
	return out, err
}

// GetDomainCache calls GET .../domains/{domain}/cache.
func (c *Client) GetDomainCache(ctx context.Context, name, domain string) (CachePolicyResource, error) {
	var out CachePolicyResource
	err := c.do(ctx, http.MethodGet, domainPolicyPath(name, domain, trafficpolicy.KindCache), nil, &out)
	return out, err
}

// SetDomainCache calls PUT .../domains/{domain}/cache.
func (c *Client) SetDomainCache(ctx context.Context, name, domain string, spec trafficpolicy.Cache) (CachePolicyResource, error) {
	var out CachePolicyResource
	err := c.do(ctx, http.MethodPut, domainPolicyPath(name, domain, trafficpolicy.KindCache), spec, &out)
	return out, err
}

// ClearDomainCache calls DELETE .../domains/{domain}/cache.
func (c *Client) ClearDomainCache(ctx context.Context, name, domain string) (CachePolicyResource, error) {
	var out CachePolicyResource
	err := c.do(ctx, http.MethodDelete, domainPolicyPath(name, domain, trafficpolicy.KindCache), nil, &out)
	return out, err
}

// GeoStatus mirrors the ingress's country source report.
type GeoStatus struct {
	Active       bool     `json:"active"`
	Sources      []string `json:"sources"`
	Header       string   `json:"header,omitempty"`
	HeaderNeeds  string   `json:"header_needs,omitempty"`
	DBPath       string   `json:"db_path,omitempty"`
	DBType       string   `json:"db_type,omitempty"`
	DBBuildEpoch uint     `json:"db_build_epoch,omitempty"`
	Error        string   `json:"error,omitempty"`
}

// CacheBucket is one minute of cache activity.
type CacheBucket struct {
	Minute int64  `json:"minute"`
	Hits   uint64 `json:"hits"`
	Misses uint64 `json:"misses"`
}

// CacheStats is one domain's in-process cache activity.
type CacheStats struct {
	Hits     uint64        `json:"hits"`
	Misses   uint64        `json:"misses"`
	Bypasses uint64        `json:"bypasses"`
	Stale    uint64        `json:"stale"`
	Entries  int           `json:"entries"`
	Bytes    int64         `json:"bytes"`
	Series   []CacheBucket `json:"series"`
}

// PolicyLimits are the instance caps for traffic controls.
type PolicyLimits struct {
	MaxHeaderRules    int   `json:"max_header_rules"`
	MaxForwarders     int   `json:"max_forwarders"`
	MaxCacheRules     int   `json:"max_cache_rules"`
	MaxRegexLen       int   `json:"max_regex_len"`
	MaxHeaderValueLen int   `json:"max_header_value_len"`
	CacheMaxObject    int64 `json:"cache_max_object_bytes"`
	CacheTotalBytes   int64 `json:"cache_total_bytes"`
	ForwardTimeoutSec int   `json:"forward_timeout_seconds"`
}

// DomainPoliciesResource is GET .../domains/{domain}/policies.
type DomainPoliciesResource struct {
	Domain    string                       `json:"domain"`
	App       string                       `json:"app"`
	Policy    trafficpolicy.Policy         `json:"policy"`
	UpdatedAt map[string]string            `json:"updated_at"`
	TLSReal   bool                         `json:"tls_real"`
	Geo       GeoStatus                    `json:"geo"`
	Cache     CacheStats                   `json:"cache"`
	Limits    PolicyLimits                 `json:"limits"`
	Preview   []trafficpolicy.Step         `json:"preview"`
	Warnings  []string                     `json:"warnings,omitempty"`
	Security  trafficpolicy.SecurityPreset `json:"security_preset"`
}

// GetDomainPolicies calls GET .../domains/{domain}/policies.
func (c *Client) GetDomainPolicies(ctx context.Context, name, domain string) (DomainPoliciesResource, error) {
	var out DomainPoliciesResource
	err := c.do(ctx, http.MethodGet, domainPolicyPath(name, domain, "policies"), nil, &out)
	return out, err
}

// PolicyPreviewRequest overlays draft sections and asks what happens to
// one request and to sample URLs.
type PolicyPreviewRequest struct {
	Policy          *trafficpolicy.Policy `json:"policy,omitempty"`
	Method          string                `json:"method,omitempty"`
	Path            string                `json:"path,omitempty"`
	URLs            []string              `json:"urls,omitempty"`
	CanonicalPreset string                `json:"canonical_preset,omitempty"`
}

// PolicyPreviewHops is one sample URL's redirect chain.
type PolicyPreviewHops struct {
	URL   string              `json:"url"`
	Hops  []trafficpolicy.Hop `json:"hops"`
	Error string              `json:"error,omitempty"`
}

// PolicyPreviewResponse is POST .../policies/preview's result.
type PolicyPreviewResponse struct {
	Steps  []trafficpolicy.Step       `json:"steps"`
	Hops   []PolicyPreviewHops        `json:"hops,omitempty"`
	Errors []trafficpolicy.FieldError `json:"errors,omitempty"`
}

// PreviewDomainPolicies calls POST .../domains/{domain}/policies/preview.
func (c *Client) PreviewDomainPolicies(ctx context.Context, name, domain string, req PolicyPreviewRequest) (PolicyPreviewResponse, error) {
	var out PolicyPreviewResponse
	err := c.do(ctx, http.MethodPost, domainPolicyPath(name, domain, "policies/preview"), req, &out)
	return out, err
}

// PurgeCacheRequest selects what to purge: scope url, prefix or all.
type PurgeCacheRequest struct {
	Scope string `json:"scope"`
	Value string `json:"value,omitempty"`
}

// PurgeCacheResponse reports how many entries were removed.
type PurgeCacheResponse struct {
	Domain string `json:"domain"`
	Purged int    `json:"purged"`
}

// PurgeDomainCache calls POST .../domains/{domain}/cache/purge.
func (c *Client) PurgeDomainCache(ctx context.Context, name, domain string, req PurgeCacheRequest) (PurgeCacheResponse, error) {
	var out PurgeCacheResponse
	err := c.do(ctx, http.MethodPost, domainPolicyPath(name, domain, "cache/purge"), req, &out)
	return out, err
}

// GetDomainCacheStats calls GET .../domains/{domain}/cache/stats.
func (c *Client) GetDomainCacheStats(ctx context.Context, name, domain string) (CacheStats, error) {
	var out CacheStats
	err := c.do(ctx, http.MethodGet, domainPolicyPath(name, domain, "cache/stats"), nil, &out)
	return out, err
}

// RedirectEffective explains who redirects plain HTTP to HTTPS.
type RedirectEffective struct {
	HandledBy             string `json:"handled_by"`
	Explanation           string `json:"explanation"`
	HTTPSPort             int    `json:"https_port"`
	TLSTerminatedUpstream bool   `json:"tls_terminated_upstream"`
	IngressHTTPRedirect   bool   `json:"ingress_http_redirect"`
}

// RedirectDomainRow is one of the app's domains on the Redirects tab.
type RedirectDomainRow struct {
	Domain      string                      `json:"domain"`
	Redirect    *trafficpolicy.HostRedirect `json:"redirect,omitempty"`
	Maintenance bool                        `json:"maintenance"`
	Wildcard    bool                        `json:"wildcard"`
}

// CanonicalState is the www and apex setup for a domain.
type CanonicalState struct {
	Counterpart         string `json:"counterpart,omitempty"`
	CounterpartAttached bool   `json:"counterpart_attached"`
	AttachedElsewhere   string `json:"counterpart_app,omitempty"`
	Preset              string `json:"preset"`
	DNSHint             string `json:"dns_hint,omitempty"`
	Unavailable         string `json:"unavailable,omitempty"`
}

// DomainRedirectsResource is GET .../domains/{domain}/redirects.
type DomainRedirectsResource struct {
	Domain     string                  `json:"domain"`
	Configured bool                    `json:"configured"`
	Settings   trafficpolicy.Redirects `json:"settings"`
	Effective  RedirectEffective       `json:"effective"`
	AppDomains []RedirectDomainRow     `json:"app_domains"`
	Canonical  CanonicalState          `json:"canonical"`
	Samples    []PolicyPreviewHops     `json:"samples"`
	Warnings   []string                `json:"warnings,omitempty"`
}

// GetDomainRedirects calls GET .../domains/{domain}/redirects.
func (c *Client) GetDomainRedirects(ctx context.Context, name, domain string) (DomainRedirectsResource, error) {
	var out DomainRedirectsResource
	err := c.do(ctx, http.MethodGet, domainPolicyPath(name, domain, "redirects"), nil, &out)
	return out, err
}

// SetDomainRedirectSettings calls PUT .../domains/{domain}/redirects.
func (c *Client) SetDomainRedirectSettings(ctx context.Context, name, domain string, s trafficpolicy.Redirects) (DomainRedirectsResource, error) {
	var out DomainRedirectsResource
	err := c.do(ctx, http.MethodPut, domainPolicyPath(name, domain, "redirects"), s, &out)
	return out, err
}

// ClearDomainRedirectSettings calls DELETE .../domains/{domain}/redirects.
func (c *Client) ClearDomainRedirectSettings(ctx context.Context, name, domain string) (DomainRedirectsResource, error) {
	var out DomainRedirectsResource
	err := c.do(ctx, http.MethodDelete, domainPolicyPath(name, domain, "redirects"), nil, &out)
	return out, err
}

// CanonicalRequest applies a www and apex preset.
type CanonicalRequest struct {
	Preset     string `json:"preset"`
	StatusCode int    `json:"status_code,omitempty"`
	Attach     *bool  `json:"attach,omitempty"`
	Replace    bool   `json:"replace,omitempty"`
}

// SetDomainCanonical calls POST .../domains/{domain}/redirects/canonical.
func (c *Client) SetDomainCanonical(ctx context.Context, name, domain string, req CanonicalRequest) (DomainRedirectsResource, error) {
	var out DomainRedirectsResource
	err := c.do(ctx, http.MethodPost, domainPolicyPath(name, domain, "redirects/canonical"), req, &out)
	return out, err
}

// AliasesRequest makes the path domain the primary for every alias.
type AliasesRequest struct {
	Aliases    []string `json:"aliases"`
	StatusCode int      `json:"status_code,omitempty"`
}

// SetDomainAliases calls PUT .../domains/{domain}/redirects/aliases.
func (c *Client) SetDomainAliases(ctx context.Context, name, domain string, req AliasesRequest) (DomainRedirectsResource, error) {
	var out DomainRedirectsResource
	err := c.do(ctx, http.MethodPut, domainPolicyPath(name, domain, "redirects/aliases"), req, &out)
	return out, err
}

// PortHolder is a container already publishing a host port.
type PortHolder struct {
	Container string `json:"container"`
	Image     string `json:"image"`
}

// PortStream is one of the app's raw TCP/UDP streams.
type PortStream struct {
	ID             string       `json:"id"`
	Protocol       string       `json:"protocol"`
	HostPort       int          `json:"host_port"`
	ContainerPort  int          `json:"container_port"`
	Target         string       `json:"target"`
	OpenToAll      bool         `json:"open_to_all"`
	AllowedSources []string     `json:"allowed_sources"`
	Conflicts      []string     `json:"conflicts"`
	Holders        []PortHolder `json:"holders"`
}

// DomainPortsResource is GET .../domains/{domain}/ports.
type DomainPortsResource struct {
	App             string       `json:"app"`
	Domain          string       `json:"domain"`
	PublicHTTPSPort int          `json:"public_https_port"`
	Streams         []PortStream `json:"streams"`
	DetectionNote   string       `json:"detection_note,omitempty"`
}

// GetDomainPorts calls GET .../domains/{domain}/ports.
func (c *Client) GetDomainPorts(ctx context.Context, name, domain string) (DomainPortsResource, error) {
	var out DomainPortsResource
	err := c.do(ctx, http.MethodGet, domainPolicyPath(name, domain, "ports"), nil, &out)
	return out, err
}

// RestrictDomainPort calls PUT .../ports/{port}/restrict: only sources may
// reach the port.
func (c *Client) RestrictDomainPort(ctx context.Context, name, domain string, port int, sources []string) (DomainPortsResource, error) {
	var out DomainPortsResource
	body := struct {
		Sources []string `json:"sources"`
	}{sources}
	err := c.do(ctx, http.MethodPut, domainPolicyPath(name, domain, "ports/"+strconv.Itoa(port)+"/restrict"), body, &out)
	return out, err
}

// UnrestrictDomainPort calls DELETE .../ports/{port}/restrict.
func (c *Client) UnrestrictDomainPort(ctx context.Context, name, domain string, port int) (DomainPortsResource, error) {
	var out DomainPortsResource
	err := c.do(ctx, http.MethodDelete, domainPolicyPath(name, domain, "ports/"+strconv.Itoa(port)+"/restrict"), nil, &out)
	return out, err
}

// GeoIPLookupResource is GET /api/v1/system/geoip.
type GeoIPLookupResource struct {
	Status  GeoStatus `json:"status"`
	IP      string    `json:"ip,omitempty"`
	Country string    `json:"country,omitempty"`
	Source  string    `json:"source,omitempty"`
	Private bool      `json:"private,omitempty"`
	Note    string    `json:"note,omitempty"`
}

// GeoIPLookup calls GET /api/v1/system/geoip, with ?ip= when ip is set.
func (c *Client) GeoIPLookup(ctx context.Context, ip string) (GeoIPLookupResource, error) {
	var out GeoIPLookupResource
	path := "/api/v1/system/geoip"
	if ip != "" {
		path += "?ip=" + url.QueryEscape(ip)
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
