package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// PluginCommand is a CLI subcommand a plugin contributes through its manifest.
type PluginCommand struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Panel       string `json:"panel"`
	Action      string `json:"action"`
}

// PluginPanelColumn names one table column.
type PluginPanelColumn struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// PluginPanelPoint is one labelled chart value.
type PluginPanelPoint struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

// PluginWidgetView is one widget of a panel after the control plane bound it to plugin data.
type PluginWidgetView struct {
	Type    string              `json:"type"`
	Title   string              `json:"title"`
	Error   string              `json:"error,omitempty"`
	Value   string              `json:"value,omitempty"`
	Unit    string              `json:"unit,omitempty"`
	State   string              `json:"state,omitempty"`
	Text    string              `json:"text,omitempty"`
	Columns []PluginPanelColumn `json:"columns,omitempty"`
	Rows    [][]string          `json:"rows,omitempty"`
	Items   []string            `json:"items,omitempty"`
	Points  []PluginPanelPoint  `json:"points,omitempty"`
	Action  string              `json:"action,omitempty"`
	Label   string              `json:"label,omitempty"`
}

// PluginPanelView is a panel's bound data.
type PluginPanelView struct {
	Widgets []PluginWidgetView `json:"widgets"`
}

// PluginActionResult is the plugin's answer to a button press.
type PluginActionResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func pluginPanelPath(name, panel string) string {
	return "/api/v1/plugins/" + PathEscape(name) + "/panels/" + PathEscape(panel)
}

// PluginPanelData calls GET /api/v1/plugins/{name}/panels/{panel}/data.
func (c *Client) PluginPanelData(ctx context.Context, name, panel, appID string) (PluginPanelView, error) {
	path := pluginPanelPath(name, panel) + "/data"
	if appID != "" {
		path += "?app_id=" + url.QueryEscape(appID)
	}
	var out PluginPanelView
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// PluginPanelAction calls POST /api/v1/plugins/{name}/panels/{panel}/actions/{action}.
func (c *Client) PluginPanelAction(ctx context.Context, name, panel, action, appID string) (PluginActionResult, error) {
	body := map[string]string{}
	if appID != "" {
		body["app_id"] = appID
	}
	var out PluginActionResult
	err := c.do(ctx, http.MethodPost, pluginPanelPath(name, panel)+"/actions/"+PathEscape(action), body, &out)
	return out, err
}
