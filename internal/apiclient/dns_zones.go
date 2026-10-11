package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// DNSZone mirrors internal/dnszones.Zone.
type DNSZone struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Provider    string   `json:"provider"`
	Status      string   `json:"status,omitempty"`
	RecordCount *int     `json:"record_count,omitempty"`
	NameServers []string `json:"name_servers"`
	Private     bool     `json:"private,omitempty"`
	ModifiedAt  string   `json:"modified_at,omitempty"`
}

// DNSCapabilities mirrors internal/dnszones.Capabilities.
type DNSCapabilities struct {
	Proxied      bool `json:"proxied"`
	Routing      bool `json:"routing"`
	HealthChecks bool `json:"health_checks"`
	ApexCNAME    bool `json:"apex_cname"`
}

// DNSZonesList is GET /api/v1/dns/zones.
type DNSZonesList struct {
	Provider     string          `json:"provider"`
	Providers    []string        `json:"providers"`
	Capabilities DNSCapabilities `json:"capabilities"`
	Zones        []DNSZone       `json:"zones"`
}

// DNSRecordSet mirrors internal/dnszones.RecordSet.
type DNSRecordSet struct {
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	TTL           int      `json:"ttl"`
	Values        []string `json:"values"`
	Proxied       bool     `json:"proxied,omitempty"`
	Routing       string   `json:"routing,omitempty"`
	SetIdentifier string   `json:"set_identifier,omitempty"`
	Weight        *int64   `json:"weight,omitempty"`
	Failover      string   `json:"failover,omitempty"`
	HealthCheckID string   `json:"health_check_id,omitempty"`
	Managed       bool     `json:"managed,omitempty"`
	Alias         *struct {
		DNSName string `json:"dns_name"`
	} `json:"alias,omitempty"`
}

// DNSRecordKey identifies a record set.
type DNSRecordKey struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	SetIdentifier string `json:"set_identifier,omitempty"`
}

// DNSRecordsList is GET /api/v1/dns/zones/{zone}/records.
type DNSRecordsList struct {
	Zone     DNSZone        `json:"zone"`
	Provider string         `json:"provider"`
	Records  []DNSRecordSet `json:"records"`
	Total    int            `json:"total"`
}

// DNSIssue is a conflict or warning on a record set.
type DNSIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// DNSRecordWrite is the create and update response.
type DNSRecordWrite struct {
	Record DNSRecordSet `json:"record"`
	Issues []DNSIssue   `json:"issues,omitempty"`
}

// DNSChange is one planned change.
type DNSChange struct {
	Action string       `json:"action"`
	Set    DNSRecordSet `json:"set"`
	Issues []DNSIssue   `json:"issues,omitempty"`
}

// DNSImportResult is the import and template response.
type DNSImportResult struct {
	Plan struct {
		Changes []DNSChange    `json:"changes"`
		Summary map[string]int `json:"summary"`
		Blocked bool           `json:"blocked"`
	} `json:"plan"`
	Warnings []string `json:"warnings,omitempty"`
	Errors   []string `json:"errors,omitempty"`
	Applied  int      `json:"applied"`
}

// DNSImportRequest is POST .../records/import.
type DNSImportRequest struct {
	Format  string `json:"format"`
	Content string `json:"content"`
	Replace bool   `json:"replace,omitempty"`
	Apply   bool   `json:"apply,omitempty"`
	Confirm string `json:"confirm,omitempty"`
}

// DNSResolverDelegation is one resolver's NS answer.
type DNSResolverDelegation struct {
	Server      string   `json:"server"`
	NameServers []string `json:"name_servers,omitempty"`
	Verdict     string   `json:"verdict"`
	Error       string   `json:"error,omitempty"`
}

// DNSDelegation is GET .../delegation.
type DNSDelegation struct {
	Delegation struct {
		Domain    string                  `json:"domain"`
		State     string                  `json:"state"`
		Expected  []string                `json:"expected"`
		Resolvers []DNSResolverDelegation `json:"resolvers"`
		Elsewhere []string                `json:"elsewhere,omitempty"`
	} `json:"delegation"`
	CheckedAt string `json:"checked_at"`
}

// DNSPropagationAnswer is one server's answer in a check.
type DNSPropagationAnswer struct {
	Server  string   `json:"server"`
	Source  string   `json:"source"`
	Values  []string `json:"values,omitempty"`
	TTL     uint32   `json:"ttl"`
	CNAME   string   `json:"cname,omitempty"`
	Error   string   `json:"error,omitempty"`
	Matches *bool    `json:"matches,omitempty"`
}

// DNSPropagation is GET /api/v1/dns/check.
type DNSPropagation struct {
	Name     string                 `json:"name"`
	Type     string                 `json:"type"`
	Expected []string               `json:"expected,omitempty"`
	Answers  []DNSPropagationAnswer `json:"answers"`
	Agree    bool                   `json:"agree"`
}

func dnsPath(provider, path string, q url.Values) string {
	if q == nil {
		q = url.Values{}
	}
	if provider != "" {
		q.Set("provider", provider)
	}
	if enc := q.Encode(); enc != "" {
		return "/api/v1/dns" + path + "?" + enc
	}
	return "/api/v1/dns" + path
}

func zonePath(zone, suffix string) string { return "/zones/" + PathEscape(zone) + suffix }

// ListDNSZones calls GET /api/v1/dns/zones.
func (c *Client) ListDNSZones(ctx context.Context, provider string) (DNSZonesList, error) {
	var out DNSZonesList
	err := c.do(ctx, http.MethodGet, dnsPath(provider, "/zones", nil), nil, &out)
	return out, err
}

// CreateDNSZone calls POST /api/v1/dns/zones.
func (c *Client) CreateDNSZone(ctx context.Context, provider, name, accountID string) (DNSZone, error) {
	var out DNSZone
	err := c.do(ctx, http.MethodPost, dnsPath(provider, "/zones", nil), map[string]string{"name": name, "account_id": accountID}, &out)
	return out, err
}

// DeleteDNSZone calls DELETE /api/v1/dns/zones/{zone}.
func (c *Client) DeleteDNSZone(ctx context.Context, provider, zone, confirm string, force bool) error {
	return c.do(ctx, http.MethodDelete, dnsPath(provider, zonePath(zone, ""), nil), map[string]any{"confirm": confirm, "force": force}, nil)
}

// DNSZoneNameServers calls GET /api/v1/dns/zones/{zone}/nameservers.
func (c *Client) DNSZoneNameServers(ctx context.Context, provider, zone string) ([]string, error) {
	var out struct {
		NameServers []string `json:"name_servers"`
	}
	err := c.do(ctx, http.MethodGet, dnsPath(provider, zonePath(zone, "/nameservers"), nil), nil, &out)
	return out.NameServers, err
}

// DNSZoneDelegation calls GET /api/v1/dns/zones/{zone}/delegation.
func (c *Client) DNSZoneDelegation(ctx context.Context, provider, zone string) (DNSDelegation, error) {
	var out DNSDelegation
	err := c.do(ctx, http.MethodGet, dnsPath(provider, zonePath(zone, "/delegation"), nil), nil, &out)
	return out, err
}

// ListDNSZoneRecords calls GET /api/v1/dns/zones/{zone}/records.
func (c *Client) ListDNSZoneRecords(ctx context.Context, provider, zone, typ, query string) (DNSRecordsList, error) {
	q := url.Values{}
	if typ != "" {
		q.Set("type", typ)
	}
	if query != "" {
		q.Set("q", query)
	}
	var out DNSRecordsList
	err := c.do(ctx, http.MethodGet, dnsPath(provider, zonePath(zone, "/records"), q), nil, &out)
	return out, err
}

// CreateDNSZoneRecord calls POST /api/v1/dns/zones/{zone}/records.
func (c *Client) CreateDNSZoneRecord(ctx context.Context, provider, zone string, rs DNSRecordSet) (DNSRecordWrite, error) {
	var out DNSRecordWrite
	err := c.do(ctx, http.MethodPost, dnsPath(provider, zonePath(zone, "/records"), nil), rs, &out)
	return out, err
}

// UpdateDNSZoneRecord calls PUT /api/v1/dns/zones/{zone}/records.
func (c *Client) UpdateDNSZoneRecord(ctx context.Context, provider, zone string, original DNSRecordKey, rs DNSRecordSet) (DNSRecordWrite, error) {
	var out DNSRecordWrite
	err := c.do(ctx, http.MethodPut, dnsPath(provider, zonePath(zone, "/records"), nil), map[string]any{"original": original, "record": rs}, &out)
	return out, err
}

// DeleteDNSZoneRecord calls DELETE /api/v1/dns/zones/{zone}/records.
func (c *Client) DeleteDNSZoneRecord(ctx context.Context, provider, zone string, k DNSRecordKey) error {
	q := url.Values{"name": {k.Name}, "type": {k.Type}}
	if k.SetIdentifier != "" {
		q.Set("set_identifier", k.SetIdentifier)
	}
	return c.do(ctx, http.MethodDelete, dnsPath(provider, zonePath(zone, "/records"), q), nil, nil)
}

// ImportDNSZoneRecords calls POST /api/v1/dns/zones/{zone}/records/import.
func (c *Client) ImportDNSZoneRecords(ctx context.Context, provider, zone string, req DNSImportRequest) (DNSImportResult, error) {
	var out DNSImportResult
	err := c.do(ctx, http.MethodPost, dnsPath(provider, zonePath(zone, "/records/import"), nil), req, &out)
	return out, err
}

// ExportDNSZoneRecords calls GET /api/v1/dns/zones/{zone}/records/export.
func (c *Client) ExportDNSZoneRecords(ctx context.Context, provider, zone, format string) ([]byte, error) {
	return c.downloadRaw(ctx, dnsPath(provider, zonePath(zone, "/records/export"), url.Values{"format": {format}}))
}

// DNSHealthCheck mirrors internal/dnszones.HealthCheck.
type DNSHealthCheck struct {
	ID               string `json:"id,omitempty"`
	Type             string `json:"type"`
	IPAddress        string `json:"ip_address,omitempty"`
	FQDN             string `json:"fqdn,omitempty"`
	Port             int32  `json:"port,omitempty"`
	ResourcePath     string `json:"resource_path,omitempty"`
	RequestInterval  int32  `json:"request_interval,omitempty"`
	FailureThreshold int32  `json:"failure_threshold,omitempty"`
}

// ApplyDNSTemplate calls POST /api/v1/dns/zones/{zone}/templates/{id}.
func (c *Client) ApplyDNSTemplate(ctx context.Context, provider, zone, id string, params map[string]string, apply bool) (DNSImportResult, error) {
	var out DNSImportResult
	err := c.do(ctx, http.MethodPost, dnsPath(provider, zonePath(zone, "/templates/"+PathEscape(id)), nil), map[string]any{"params": params, "apply": apply}, &out)
	return out, err
}

// ListDNSHealthChecks calls GET /api/v1/dns/health-checks.
func (c *Client) ListDNSHealthChecks(ctx context.Context) ([]DNSHealthCheck, error) {
	var out []DNSHealthCheck
	err := c.do(ctx, http.MethodGet, dnsPath("", "/health-checks", nil), nil, &out)
	return out, err
}

// CreateDNSHealthCheck calls POST /api/v1/dns/health-checks.
func (c *Client) CreateDNSHealthCheck(ctx context.Context, hc DNSHealthCheck) (DNSHealthCheck, error) {
	var out DNSHealthCheck
	err := c.do(ctx, http.MethodPost, dnsPath("", "/health-checks", nil), hc, &out)
	return out, err
}

// DeleteDNSHealthCheck calls DELETE /api/v1/dns/health-checks/{id}.
func (c *Client) DeleteDNSHealthCheck(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, dnsPath("", "/health-checks/"+PathEscape(id), nil), nil, nil)
}

// CheckDNS calls GET /api/v1/dns/check.
func (c *Client) CheckDNS(ctx context.Context, name, typ, zone string) (DNSPropagation, error) {
	q := url.Values{"name": {name}, "type": {typ}}
	if zone != "" {
		q.Set("zone", zone)
	}
	var out DNSPropagation
	err := c.do(ctx, http.MethodGet, dnsPath("", "/check", q), nil, &out)
	return out, err
}
