package apiclient

import (
	"context"
	"net/http"
)

// ObservabilityChoice is one allowed value of a select field.
type ObservabilityChoice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ObservabilityField is one input of a preset form.
type ObservabilityField struct {
	Key         string                `json:"key"`
	Label       string                `json:"label"`
	Type        string                `json:"type,omitempty"`
	Secret      bool                  `json:"secret,omitempty"`
	Required    bool                  `json:"required,omitempty"`
	Default     string                `json:"default,omitempty"`
	Placeholder string                `json:"placeholder,omitempty"`
	Help        string                `json:"help,omitempty"`
	Choices     []ObservabilityChoice `json:"choices,omitempty"`
}

// ObservabilityPresetSignal names a signal a preset can export.
type ObservabilityPresetSignal struct {
	Signal  string `json:"signal"`
	Default bool   `json:"default"`
}

// ObservabilityPreset is one provider entry of the catalog.
type ObservabilityPreset struct {
	ID          string                      `json:"id"`
	Name        string                      `json:"name"`
	Description string                      `json:"description"`
	DocsAnchor  string                      `json:"docs_anchor"`
	Fields      []ObservabilityField        `json:"fields"`
	Signals     []ObservabilityPresetSignal `json:"signals"`
}

// ObservabilityPresetCatalog is the body of GET /api/v1/observability/presets.
type ObservabilityPresetCatalog struct {
	Presets []ObservabilityPreset `json:"presets"`
}

// ObservabilitySignal is one delivered signal of an export. It never carries a credential.
type ObservabilitySignal struct {
	Signal        string   `json:"signal"`
	DestinationID string   `json:"destination_id"`
	Kind          string   `json:"kind"`
	Endpoint      string   `json:"endpoint"`
	Enabled       bool     `json:"enabled"`
	HeaderNames   []string `json:"header_names"`
	HasCredential bool     `json:"has_credential"`
	LastSuccessAt string   `json:"last_success_at"`
	LastError     string   `json:"last_error"`
	SentTotal     int64    `json:"sent_total"`
	CircuitOpen   bool     `json:"circuit_open"`
}

// ObservabilityExport is one export made from a preset.
type ObservabilityExport struct {
	ID         string                `json:"id"`
	Preset     string                `json:"preset"`
	PresetName string                `json:"preset_name"`
	Signals    []ObservabilitySignal `json:"signals"`
}

// CreateObservabilityExportRequest is the body of POST /api/v1/observability/exports.
type CreateObservabilityExportRequest struct {
	Preset  string            `json:"preset"`
	Name    string            `json:"name"`
	Fields  map[string]string `json:"fields"`
	Signals []string          `json:"signals,omitempty"`
}

// ObservabilityTestResult is the outcome of one signal probe.
type ObservabilityTestResult struct {
	Signal string `json:"signal"`
	OK     bool   `json:"ok"`
	Status int    `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
	Hint   string `json:"hint,omitempty"`
	Fix    string `json:"fix,omitempty"`
}

// ObservabilityTestResponse is the body of POST .../exports/{id}/test.
type ObservabilityTestResponse struct {
	ID      string                    `json:"id"`
	OK      bool                      `json:"ok"`
	Results []ObservabilityTestResult `json:"results"`
}

func observabilityExportPath(id string) string {
	return "/api/v1/observability/exports/" + PathEscape(id)
}

// ListObservabilityPresets calls GET /api/v1/observability/presets.
func (c *Client) ListObservabilityPresets(ctx context.Context) (ObservabilityPresetCatalog, error) {
	var out ObservabilityPresetCatalog
	err := c.do(ctx, http.MethodGet, "/api/v1/observability/presets", nil, &out)
	return out, err
}

// ListObservabilityExports calls GET /api/v1/observability/exports.
func (c *Client) ListObservabilityExports(ctx context.Context) ([]ObservabilityExport, error) {
	var out []ObservabilityExport
	err := c.do(ctx, http.MethodGet, "/api/v1/observability/exports", nil, &out)
	return out, err
}

// CreateObservabilityExport calls POST /api/v1/observability/exports.
func (c *Client) CreateObservabilityExport(ctx context.Context, req CreateObservabilityExportRequest) (ObservabilityExport, error) {
	var out ObservabilityExport
	err := c.do(ctx, http.MethodPost, "/api/v1/observability/exports", req, &out)
	return out, err
}

// TestObservabilityExport calls POST /api/v1/observability/exports/{id}/test.
func (c *Client) TestObservabilityExport(ctx context.Context, id string) (ObservabilityTestResponse, error) {
	var out ObservabilityTestResponse
	err := c.do(ctx, http.MethodPost, observabilityExportPath(id)+"/test", nil, &out)
	return out, err
}

// DeleteObservabilityExport calls DELETE /api/v1/observability/exports/{id}.
func (c *Client) DeleteObservabilityExport(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, observabilityExportPath(id), nil, nil)
}
