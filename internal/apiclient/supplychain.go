package apiclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// VulnCounts is the number of findings per severity.
type VulnCounts struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Unknown  int `json:"unknown"`
}

// SBOMPackage is one component of an SBOM.
type SBOMPackage struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Type    string `json:"type,omitempty"`
	License string `json:"license,omitempty"`
}

// SBOMTypeCount is how many packages are of a type.
type SBOMTypeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// SBOMLicenseCount is how many packages carry a license.
type SBOMLicenseCount struct {
	License string `json:"license"`
	Count   int    `json:"count"`
}

// SBOMSummary is GET .../deployments/{id}/sbom's body.
type SBOMSummary struct {
	DeploymentID string             `json:"deployment_id"`
	Format       string             `json:"format"`
	PackageCount int                `json:"package_count"`
	Types        []SBOMTypeCount    `json:"types"`
	Licenses     []SBOMLicenseCount `json:"licenses"`
	Unlicensed   int                `json:"unlicensed"`
	TopPackages  []SBOMPackage      `json:"top_packages"`
	Provenance   bool               `json:"provenance"`
	Available    bool               `json:"available"`
	Bytes        int64              `json:"bytes"`
	GeneratedAt  time.Time          `json:"generated_at"`
	DownloadURL  string             `json:"download_url,omitempty"`
}

// Vulnerability is one scanner finding.
type Vulnerability struct {
	ID           string `json:"id"`
	Package      string `json:"package"`
	Version      string `json:"version,omitempty"`
	FixedVersion string `json:"fixed_version,omitempty"`
	Severity     string `json:"severity"`
	Title        string `json:"title,omitempty"`
}

// VulnScan is the scan half of a VulnReport.
type VulnScan struct {
	Status    string          `json:"status"`
	Scanner   string          `json:"scanner,omitempty"`
	ScannedAt *time.Time      `json:"scanned_at,omitempty"`
	Error     string          `json:"error,omitempty"`
	Counts    *VulnCounts     `json:"counts,omitempty"`
	Fixable   int             `json:"fixable"`
	Top       []Vulnerability `json:"top"`
}

// VulnGate is the gate decision of a VulnReport.
type VulnGate struct {
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
}

// VulnReport is GET .../deployments/{id}/vulnerabilities' body.
type VulnReport struct {
	DeploymentID string    `json:"deployment_id"`
	Scan         VulnScan  `json:"scan"`
	Gate         *VulnGate `json:"gate,omitempty"`
}

// SupplyChainSettings is GET /api/v1/apps/{name}/supply-chain's body.
type SupplyChainSettings struct {
	App             string     `json:"app"`
	ScanEnabled     bool       `json:"scan_enabled"`
	ScanGate        string     `json:"scan_gate"`
	ServerEnabled   bool       `json:"server_enabled"`
	BuildAttest     bool       `json:"build_attest"`
	Scanner         string     `json:"scanner"`
	ScannerImage    string     `json:"scanner_image"`
	OverrideArmed   bool       `json:"override_armed"`
	OverrideReason  string     `json:"override_reason,omitempty"`
	OverrideExpires *time.Time `json:"override_expires_at,omitempty"`
}

// SupplyChainSettingsRequest is PUT .../supply-chain's body; nil fields stay unchanged.
type SupplyChainSettingsRequest struct {
	ScanEnabled *bool   `json:"scan_enabled,omitempty"`
	ScanGate    *string `json:"scan_gate,omitempty"`
}

func supplyChainPath(name string) string { return "/api/v1/apps/" + PathEscape(name) + "/supply-chain" }

func deploymentPath(name, id, leaf string) string {
	return "/api/v1/apps/" + PathEscape(name) + "/deployments/" + PathEscape(id) + "/" + leaf
}

// GetSupplyChain calls GET /api/v1/apps/{name}/supply-chain.
func (c *Client) GetSupplyChain(ctx context.Context, name string) (SupplyChainSettings, error) {
	var out SupplyChainSettings
	err := c.do(ctx, http.MethodGet, supplyChainPath(name), nil, &out)
	return out, err
}

// SetSupplyChain calls PUT /api/v1/apps/{name}/supply-chain.
func (c *Client) SetSupplyChain(ctx context.Context, name string, req SupplyChainSettingsRequest) (SupplyChainSettings, error) {
	var out SupplyChainSettings
	err := c.do(ctx, http.MethodPut, supplyChainPath(name), req, &out)
	return out, err
}

// OverrideSupplyChainGate calls POST /api/v1/apps/{name}/supply-chain/override.
func (c *Client) OverrideSupplyChainGate(ctx context.Context, name, reason string) (SupplyChainSettings, error) {
	var out SupplyChainSettings
	err := c.do(ctx, http.MethodPost, supplyChainPath(name)+"/override", map[string]string{"reason": reason}, &out)
	return out, err
}

// GetSBOM calls GET /api/v1/apps/{name}/deployments/{id}/sbom.
func (c *Client) GetSBOM(ctx context.Context, name, id string) (SBOMSummary, error) {
	var out SBOMSummary
	err := c.do(ctx, http.MethodGet, deploymentPath(name, id, "sbom"), nil, &out)
	return out, err
}

// DownloadSBOM fetches the raw SBOM document of one deployment.
func (c *Client) DownloadSBOM(ctx context.Context, name, id string) ([]byte, error) {
	path := deploymentPath(name, id, "sbom") + "?download=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil) //nolint:gosec // c.baseURL is the operator-supplied API target
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	resp, err := c.hc.Do(req) //nolint:gosec // same target as above
	if err != nil {
		return nil, fmt.Errorf("request GET %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return nil, &APIError{StatusCode: resp.StatusCode, Message: ExtractErrorMessage(data), RetryAfter: retryAfterHeader(resp.Header)}
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	return data, nil
}

// GetVulnerabilities calls GET /api/v1/apps/{name}/deployments/{id}/vulnerabilities.
func (c *Client) GetVulnerabilities(ctx context.Context, name, id string) (VulnReport, error) {
	var out VulnReport
	err := c.do(ctx, http.MethodGet, deploymentPath(name, id, "vulnerabilities"), nil, &out)
	return out, err
}

// ScanDeployment calls POST /api/v1/apps/{name}/deployments/{id}/scan.
func (c *Client) ScanDeployment(ctx context.Context, name, id string) (VulnReport, error) {
	var out VulnReport
	err := c.do(ctx, http.MethodPost, deploymentPath(name, id, "scan"), nil, &out)
	return out, err
}
