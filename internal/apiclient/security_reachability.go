package apiclient

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// ReachFix mirrors internal/exposure's ReachFix.
type ReachFix struct {
	Kind     string            `json:"kind"`
	Method   string            `json:"method,omitempty"`
	Path     string            `json:"path,omitempty"`
	Body     map[string]string `json:"body,omitempty"`
	Link     string            `json:"link,omitempty"`
	Restarts bool              `json:"restarts,omitempty"`
}

// ReachRef mirrors internal/exposure's ReachRef.
type ReachRef struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

// ReachFinding mirrors internal/exposure's ReachFinding.
type ReachFinding struct {
	ID       string            `json:"id"`
	Kind     string            `json:"kind"`
	Severity string            `json:"severity"`
	Subject  ReachRef          `json:"subject"`
	Node     string            `json:"node,omitempty"`
	Port     int               `json:"port,omitempty"`
	Params   map[string]string `json:"params,omitempty"`
	Fix      *ReachFix         `json:"fix,omitempty"`
}

// ReachEndpoint mirrors internal/exposure's ReachEndpoint.
type ReachEndpoint struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	Class string `json:"class,omitempty"`
}

// ReachSubject mirrors internal/exposure's ReachSubject.
type ReachSubject struct {
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	Endpoints []ReachEndpoint `json:"endpoints"`
	Findings  int             `json:"findings"`
}

// ReachCounts mirrors internal/exposure's ReachCounts.
type ReachCounts struct {
	High   int `json:"high"`
	Medium int `json:"medium"`
	Low    int `json:"low"`
	Info   int `json:"info"`
}

// ReachReport mirrors internal/exposure's ReachReport.
type ReachReport struct {
	GeneratedAt     string         `json:"generated_at"`
	Counts          ReachCounts    `json:"counts"`
	Subjects        []ReachSubject `json:"subjects"`
	Findings        []ReachFinding `json:"findings"`
	RulesReadable   bool           `json:"rules_readable"`
	RestrictedPorts int            `json:"restricted_ports"`
}

// GetSecurityReachability calls GET /api/v1/security/reachability (root); refresh re-measures first.
func (c *Client) GetSecurityReachability(ctx context.Context, refresh bool) (ReachReport, error) {
	path := "/api/v1/security/reachability"
	if refresh {
		path += "?refresh=true"
	}
	var out ReachReport
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// ApplyReachFix makes the same call the dashboard's Fix button makes for an auto fix.
func (c *Client) ApplyReachFix(ctx context.Context, fix ReachFix) error {
	if fix.Kind != "auto" || !strings.HasPrefix(fix.Path, "/api/v1/") {
		return fmt.Errorf("apiclient: finding has no automatic fix")
	}
	var body any
	if len(fix.Body) > 0 {
		body = fix.Body
	}
	return c.do(ctx, fix.Method, fix.Path, body, nil)
}
