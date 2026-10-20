package apiclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DomainReachability is one entry of AppEnvironmentDomains.Reachability.
type DomainReachability struct {
	URL    string `json:"reachable_url,omitempty"`
	Reason string `json:"reachable_reason,omitempty"`
}

// TrafficDomainCounts mirrors internal/api's trafficDomainCounts.
type TrafficDomainCounts struct {
	Total          int `json:"total"`
	Live           int `json:"live"`
	Waiting        int `json:"waiting"`
	Propagating    int `json:"propagating"`
	NeedsAttention int `json:"needs_attention"`
	NotSetUp       int `json:"not_set_up"`
	Paused         int `json:"paused"`
	Unknown        int `json:"unknown"`
}

// TrafficCertCounts mirrors internal/api's trafficCertCounts.
type TrafficCertCounts struct {
	WindowDays     int `json:"window_days"`
	Expiring       int `json:"expiring"`
	Expired        int `json:"expired"`
	RenewalFailing int `json:"renewal_failing"`
}

// TrafficSummary is GET /api/v1/traffic/summary's body.
type TrafficSummary struct {
	GeneratedAt         time.Time           `json:"generated_at"`
	Domains             TrafficDomainCounts `json:"domains"`
	Certificates        TrafficCertCounts   `json:"certificates"`
	DomainsNotResolving int                 `json:"domains_not_resolving"`
	ZonesNotDelegated   *int                `json:"zones_not_delegated,omitempty"`
	RoutesWithErrors    *int                `json:"routes_with_errors,omitempty"`
	Attention           int                 `json:"attention"`
	Unavailable         []string            `json:"unavailable,omitempty"`
}

// DoctorAction mirrors domaindoctor.Action.
type DoctorAction struct {
	Kind  string            `json:"kind"`
	Label string            `json:"label"`
	API   string            `json:"api,omitempty"`
	Value string            `json:"value,omitempty"`
	Body  map[string]string `json:"body,omitempty"`
}

// DoctorRecord mirrors domaindoctor.Record.
type DoctorRecord struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// DoctorFix mirrors domaindoctor.Fix.
type DoctorFix struct {
	Summary string         `json:"summary"`
	Kind    string         `json:"kind,omitempty"`
	Action  *DoctorAction  `json:"action,omitempty"`
	Records []DoctorRecord `json:"records,omitempty"`
	CLI     string         `json:"cli,omitempty"`
}

// DoctorPropagation mirrors domaindoctor.Propagation.
type DoctorPropagation struct {
	State            string    `json:"state"`
	Detail           string    `json:"detail"`
	NextCheckSeconds int       `json:"next_check_seconds,omitempty"`
	NextCheckAt      time.Time `json:"next_check_at,omitzero"`
}

// DoctorCheck mirrors domaindoctor.Check.
type DoctorCheck struct {
	ID     string     `json:"id"`
	Title  string     `json:"title"`
	State  string     `json:"state"`
	Tier   int        `json:"tier"`
	Detail string     `json:"detail,omitempty"`
	Fix    *DoctorFix `json:"fix,omitempty"`
}

// DomainDoctorReport is POST .../domains/{domain}/doctor's body.
type DomainDoctorReport struct {
	Domain    string        `json:"domain"`
	App       string        `json:"app,omitempty"`
	CheckedAt time.Time     `json:"checked_at"`
	Status    string        `json:"status"`
	Probed    bool          `json:"probed"`
	ProbeNote string        `json:"probe_note,omitempty"`
	Checks    []DoctorCheck `json:"checks"`
	// Records are the DNS records the domain needs to reach this server.
	Records     []DoctorRecord     `json:"records,omitempty"`
	Propagation *DoctorPropagation `json:"propagation,omitempty"`
}

// ActivityActor mirrors internal/api's activityActor.
type ActivityActor struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

// ActivityObject mirrors internal/api's activityObject.
type ActivityObject struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Href string `json:"href,omitempty"`
}

// ActivityEvent is one domain timeline entry.
type ActivityEvent struct {
	ID     string         `json:"id"`
	At     time.Time      `json:"at"`
	Kind   string         `json:"kind"`
	Title  string         `json:"title"`
	Detail string         `json:"detail,omitempty"`
	Failed bool           `json:"failed,omitempty"`
	Action string         `json:"action,omitempty"`
	Actor  ActivityActor  `json:"actor"`
	Object ActivityObject `json:"object"`
}

// ActivityPage is GET /api/v1/domains/{domain}/activity's body.
type ActivityPage struct {
	Events     []ActivityEvent `json:"events"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

// TrafficSummary calls GET /api/v1/traffic/summary.
func (c *Client) TrafficSummary(ctx context.Context) (TrafficSummary, error) {
	var out TrafficSummary
	err := c.do(ctx, http.MethodGet, "/api/v1/traffic/summary", nil, &out)
	return out, err
}

// DomainDoctor calls POST /api/v1/apps/{name}/domains/{domain}/doctor.
func (c *Client) DomainDoctor(ctx context.Context, app, domain string) (DomainDoctorReport, error) {
	var out DomainDoctorReport
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(app)+"/domains/"+PathEscape(domain)+"/doctor", nil, &out)
	return out, err
}

// DomainDiagnose calls POST /api/v1/domains/{domain}/diagnose.
func (c *Client) DomainDiagnose(ctx context.Context, domain string) (DomainDoctorReport, error) {
	var out DomainDoctorReport
	err := c.do(ctx, http.MethodPost, "/api/v1/domains/"+PathEscape(domain)+"/diagnose", nil, &out)
	return out, err
}

// ApplyDoctorAction runs a request action a diagnosis offered, such as
// creating the missing DNS record. Only "METHOD /api/v1/..." actions apply.
func (c *Client) ApplyDoctorAction(ctx context.Context, a DoctorAction) error {
	method, path, ok := strings.Cut(a.API, " ")
	if !ok || a.Kind != "request" || !strings.HasPrefix(path, "/api/v1/") {
		return fmt.Errorf("apply %q: not an API request action", a.Label)
	}
	var body any
	if len(a.Body) > 0 {
		body = a.Body
	}
	if err := c.do(ctx, method, path, body, nil); err != nil {
		return fmt.Errorf("apply %q: %w", a.Label, err)
	}
	return nil
}

// DomainActivity calls GET /api/v1/domains/{domain}/activity.
func (c *Client) DomainActivity(ctx context.Context, domain string, limit int, before, actions string) (ActivityPage, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if before != "" {
		q.Set("before", before)
	}
	if actions != "" {
		q.Set("actions", actions)
	}
	path := "/api/v1/domains/" + PathEscape(domain) + "/activity"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out ActivityPage
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
