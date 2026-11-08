package apiclient

import (
	"context"
	"net/http"
)

// SpecIssue mirrors one located problem in an app.yaml document.
type SpecIssue struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// ValidateSpecResult is the response of POST /api/v1/apps/{name}/validate-spec.
type ValidateSpecResult struct {
	Valid    bool        `json:"valid"`
	Issues   []SpecIssue `json:"issues"`
	Services int         `json:"services,omitempty"`
}

// ListeningPortsResult is the response of GET /api/v1/apps/{name}/listening-ports.
type ListeningPortsResult struct {
	App       string `json:"app"`
	Port      int    `json:"port"`
	Probed    bool   `json:"probed"`
	Reason    string `json:"reason,omitempty"`
	Listening []int  `json:"listening"`
	Verdict   string `json:"verdict"`
}

// ValidateAppSpec checks an app.yaml body server side without deploying or saving it.
func (c *Client) ValidateAppSpec(ctx context.Context, app, yaml string) (ValidateSpecResult, error) {
	var out ValidateSpecResult
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(app)+"/validate-spec", map[string]string{"yaml": yaml}, &out)
	return out, err
}

// AppListeningPorts reports which container ports the running app listens on.
func (c *Client) AppListeningPorts(ctx context.Context, app string) (ListeningPortsResult, error) {
	var out ListeningPortsResult
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(app)+"/listening-ports", nil, &out)
	return out, err
}
