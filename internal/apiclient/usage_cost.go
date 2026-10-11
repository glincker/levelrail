package apiclient

import (
	"context"
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/usagecost"
)

// UsageReport is GET /api/v1/usage/report.
type UsageReport struct {
	usagecost.Report
	Currency            string            `json:"currency"`
	Mode                string            `json:"mode"`
	Basis               string            `json:"basis"`
	Labels              map[string]string `json:"labels"`
	UnpricedNodes       []string          `json:"unpriced_nodes"`
	IncludesUnallocated bool              `json:"includes_unallocated"`
	Configured          bool              `json:"configured"`
	TelemetryAvailable  bool              `json:"telemetry_available"`
	GeneratedAt         string            `json:"generated_at"`
}

// UsageNodeInfo is one node's bill and capacity in UsageSettings.
type UsageNodeInfo struct {
	NodeID         string  `json:"node_id"`
	Name           string  `json:"name"`
	MonthlyPrice   float64 `json:"monthly_price"`
	VCPUs          float64 `json:"vcpus"`
	MemoryGiB      float64 `json:"memory_gib"`
	DetectedVCPUs  float64 `json:"detected_vcpus"`
	DetectedMemGiB float64 `json:"detected_memory_gib"`
	Priced         bool    `json:"priced"`
}

// UsageSettings is GET/PUT /api/v1/usage/settings.
type UsageSettings struct {
	Settings usagecost.Settings `json:"settings"`
	Nodes    []UsageNodeInfo    `json:"nodes"`
	Note     string             `json:"note"`
}

// UsageSettingsUpdate is the PUT body; nil fields are left unchanged.
type UsageSettingsUpdate struct {
	Settings     *usagecost.Settings    `json:"settings,omitempty"`
	Nodes        *[]usagecost.NodePrice `json:"nodes,omitempty"`
	RepriceMonth bool                   `json:"reprice_month,omitempty"`
}

// UsageBudgets is GET /api/v1/usage/budgets.
type UsageBudgets struct {
	Budgets  []usagecost.BudgetStatus `json:"budgets"`
	Currency string                   `json:"currency"`
}

// UsageReportQuery selects the period of a report.
type UsageReportQuery struct {
	Period string
	From   string
	To     string
}

func (q UsageReportQuery) values() url.Values {
	v := url.Values{}
	if q.Period != "" {
		v.Set("period", q.Period)
	}
	if q.From != "" {
		v.Set("from", q.From)
	}
	if q.To != "" {
		v.Set("to", q.To)
	}
	return v
}

// GetUsageReport calls GET /api/v1/usage/report.
func (c *Client) GetUsageReport(ctx context.Context, q UsageReportQuery) (UsageReport, error) {
	path := "/api/v1/usage/report"
	if v := q.values(); len(v) > 0 {
		path += "?" + v.Encode()
	}
	var out UsageReport
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// DownloadUsageReportCSV calls GET /api/v1/usage/report?format=csv.
func (c *Client) DownloadUsageReportCSV(ctx context.Context, q UsageReportQuery) ([]byte, error) {
	v := q.values()
	v.Set("format", "csv")
	return c.downloadRaw(ctx, "/api/v1/usage/report?"+v.Encode())
}

// GetUsageSettings calls GET /api/v1/usage/settings.
func (c *Client) GetUsageSettings(ctx context.Context) (UsageSettings, error) {
	var out UsageSettings
	err := c.do(ctx, http.MethodGet, "/api/v1/usage/settings", nil, &out)
	return out, err
}

// UpdateUsageSettings calls PUT /api/v1/usage/settings.
func (c *Client) UpdateUsageSettings(ctx context.Context, u UsageSettingsUpdate) (UsageSettings, error) {
	var out UsageSettings
	err := c.do(ctx, http.MethodPut, "/api/v1/usage/settings", u, &out)
	return out, err
}

// ListUsageBudgets calls GET /api/v1/usage/budgets.
func (c *Client) ListUsageBudgets(ctx context.Context) (UsageBudgets, error) {
	var out UsageBudgets
	err := c.do(ctx, http.MethodGet, "/api/v1/usage/budgets", nil, &out)
	return out, err
}

// SetUsageBudget calls PUT /api/v1/usage/budgets.
func (c *Client) SetUsageBudget(ctx context.Context, scope, scopeID string, monthly, warnPct float64) (usagecost.Budget, error) {
	body := map[string]any{"scope": scope, "scope_id": scopeID, "monthly": monthly}
	if warnPct > 0 {
		body["warn_pct"] = warnPct
	}
	var out usagecost.Budget
	err := c.do(ctx, http.MethodPut, "/api/v1/usage/budgets", body, &out)
	return out, err
}

// DeleteUsageBudget calls DELETE /api/v1/usage/budgets/{id}.
func (c *Client) DeleteUsageBudget(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/usage/budgets/"+PathEscape(id), nil, nil)
}
