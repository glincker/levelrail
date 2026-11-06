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
	Current []string `json:"current,omitempty"`
	State   string   `json:"state,omitempty"`
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
	Message        string `json:"message"`
	WWWAttached    bool   `json:"www_attached"`
	WWWPointsHere  bool   `json:"www_points_here"`
	ApexPointsHere bool   `json:"apex_points_here"`
	WWWRedirects   bool   `json:"www_redirects"`
}

// DomainWWWPlan reads the apex and www DNS preview; empty mode or canonical
// lets the server use its live settings.
func (c *Client) DomainWWWPlan(ctx context.Context, app, domain, mode, canonical string, noRedirect bool) (WWWPlan, error) {
	q := url.Values{}
	if noRedirect {
		q.Set("redirect", "false")
	}
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

// WWWApplyRecord is one record's outcome of an apex and www apply.
type WWWApplyRecord struct {
	WWWPlanRecord
	Result  string `json:"result"`
	Message string `json:"message,omitempty"`
}

// WWWApplyUndo is one reversible effect of an apply.
type WWWApplyUndo struct {
	Kind       string `json:"kind"`
	Domain     string `json:"domain"`
	Name       string `json:"name,omitempty"`
	RecordType string `json:"record_type,omitempty"`
	Value      string `json:"value,omitempty"`
}

// WWWApply is POST .../domains/{domain}/dns/apply's body.
type WWWApply struct {
	Domain    string           `json:"domain"`
	Provider  string           `json:"provider"`
	Zone      string           `json:"zone"`
	Mode      string           `json:"mode"`
	Canonical string           `json:"canonical"`
	Applied   bool             `json:"applied"`
	Records   []WWWApplyRecord `json:"records"`
	Redirect  *struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"redirect"`
	RedirectApplied bool           `json:"redirect_applied"`
	RedirectMessage string         `json:"redirect_message"`
	Undo            []WWWApplyUndo `json:"undo"`
	RunID           string         `json:"run_id"`
	Message         string         `json:"message"`
	WWWAttached     bool           `json:"www_attached"`
	AttachedNow     bool           `json:"attached_now"`
}

// DomainWWWApply writes the apex and www records. The server refuses unless
// confirm is true.
func (c *Client) DomainWWWApply(ctx context.Context, app, domain, mode, canonical string, replace, confirm, noRedirect, attach bool) (WWWApply, error) {
	body := map[string]any{"mode": mode, "canonical": canonical, "replace": replace, "confirm": confirm}
	if noRedirect {
		body["redirect"] = false
	}
	if attach {
		body["attach"] = true
	}
	path := "/api/v1/apps/" + PathEscape(app) + "/domains/" + PathEscape(domain) + "/dns/apply"
	var out WWWApply
	err := c.do(ctx, http.MethodPost, path, body, &out)
	return out, err
}
