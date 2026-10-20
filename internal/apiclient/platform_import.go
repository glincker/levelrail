package apiclient

import (
	"context"
	"net/http"
)

// PlatformImportRequest is the body of the platform import routes. Token
// is the source platform's credential and is sent only in this body.
type PlatformImportRequest struct {
	Platform      string   `json:"platform"`
	URL           string   `json:"url"`
	Token         string   `json:"token"`
	InsecureTLS   bool     `json:"insecure_tls,omitempty"`
	AllowPrivate  bool     `json:"allow_private,omitempty"`
	AllowLoopback bool     `json:"allow_loopback,omitempty"`
	Only          []string `json:"only,omitempty"`
	Collision     string   `json:"collision,omitempty"`
}

// PlatformImportItem is one row of a platform import report.
type PlatformImportItem struct {
	Kind       string   `json:"kind"`
	SourceID   string   `json:"source_id"`
	SourceName string   `json:"source_name"`
	Target     string   `json:"target,omitempty"`
	Status     string   `json:"status"`
	Reasons    []string `json:"reasons,omitempty"`
	Manual     []string `json:"manual,omitempty"`
}

// PlatformImportReport is the discover or apply result.
type PlatformImportReport struct {
	Platform string               `json:"platform"`
	Items    []PlatformImportItem `json:"items"`
	Counts   map[string]int       `json:"counts"`
	Notes    []string             `json:"notes,omitempty"`
}

// DiscoverPlatformImport calls POST /api/v1/imports/platform/discover.
func (c *Client) DiscoverPlatformImport(ctx context.Context, req PlatformImportRequest) (PlatformImportReport, error) {
	var out PlatformImportReport
	err := c.do(ctx, http.MethodPost, "/api/v1/imports/platform/discover", req, &out)
	return out, err
}

// ApplyPlatformImport calls POST /api/v1/imports/platform/apply.
func (c *Client) ApplyPlatformImport(ctx context.Context, req PlatformImportRequest) (PlatformImportReport, error) {
	var out PlatformImportReport
	err := c.do(ctx, http.MethodPost, "/api/v1/imports/platform/apply", req, &out)
	return out, err
}
