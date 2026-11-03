package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// WWWPlanRecord is one record of the apex and www preview.
type WWWPlanRecord struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Value      string `json:"value"`
	TTLSeconds int    `json:"ttl_seconds"`
	Outcome    string `json:"outcome"`
	Replaces   []struct {
		Name  string `json:"name"`
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"replaces"`
}

// WWWPlan is GET .../domains/{domain}/dns/plan's body.
type WWWPlan struct {
	Domain    string          `json:"domain"`
	Provider  string          `json:"provider"`
	Zone      string          `json:"zone"`
	Mode      string          `json:"mode"`
	Canonical string          `json:"canonical"`
	Records   []WWWPlanRecord `json:"records"`
	Redirect  *struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"redirect"`
	Message string `json:"message"`
}

// DomainWWWPlan reads the apex and www DNS preview; empty mode or canonical
// lets the server use its live settings.
func (c *Client) DomainWWWPlan(ctx context.Context, app, domain, mode, canonical string) (WWWPlan, error) {
	q := url.Values{}
	if mode != "" {
		q.Set("mode", mode)
	}
	if canonical != "" {
		q.Set("canonical", canonical)
	}
	path := "/api/v1/apps/" + PathEscape(app) + "/domains/" + PathEscape(domain) + "/dns/plan"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out WWWPlan
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
