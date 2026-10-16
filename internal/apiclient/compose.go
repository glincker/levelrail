package apiclient

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ComposeStackService mirrors one entry of GET /api/v1/apps/{name}/compose.
type ComposeStackService struct {
	Key              string   `json:"key"`
	Name             string   `json:"name"`
	Image            string   `json:"image"`
	Port             int      `json:"port,omitempty"`
	Domains          []string `json:"domains,omitempty"`
	DependsOn        []string `json:"depends_on,omitempty"`
	DependsOnHealthy []string `json:"depends_on_healthy,omitempty"`
	Volumes          []string `json:"volumes,omitempty"`
	Replicas         int      `json:"replicas"`
	Suspended        bool     `json:"suspended,omitempty"`
	Ready            string   `json:"ready"`
	Reason           string   `json:"reason,omitempty"`
	Message          string   `json:"message,omitempty"`
	Since            string   `json:"since,omitempty"`
}

// ComposeStack mirrors GET /api/v1/apps/{name}/compose.
type ComposeStack struct {
	AppID      string                `json:"app_id"`
	Phase      string                `json:"phase"`
	Revision   int                   `json:"revision,omitempty"`
	DeployedAt string                `json:"deployed_at,omitempty"`
	Source     string                `json:"source,omitempty"`
	TemplateID string                `json:"template_id,omitempty"`
	Profiles   []string              `json:"profiles,omitempty"`
	Services   []ComposeStackService `json:"services"`
}

// ComposeRevision mirrors one compose revision.
type ComposeRevision struct {
	Revision   int      `json:"revision"`
	Source     string   `json:"source"`
	TemplateID string   `json:"template_id,omitempty"`
	Profiles   []string `json:"profiles,omitempty"`
	CreatedBy  string   `json:"created_by,omitempty"`
	CreatedAt  string   `json:"created_at"`
	Compose    string   `json:"compose,omitempty"`
}

// ComposeStackDeploy is a compose deploy response with its revision.
type ComposeStackDeploy struct {
	AppID    string        `json:"app_id"`
	Services []AppResource `json:"services"`
	Notices  []struct {
		Level   string `json:"level"`
		Message string `json:"message"`
	} `json:"notices,omitempty"`
	Revision int      `json:"revision,omitempty"`
	Skipped  []string `json:"skipped_services,omitempty"`
}

// ComposeRollbackResult mirrors POST /api/v1/apps/{name}/compose/rollback.
type ComposeRollbackResult struct {
	AppID        string             `json:"app_id"`
	RolledBackTo int                `json:"rolled_back_to"`
	Deploy       ComposeStackDeploy `json:"deploy"`
}

// ComposeDownResult mirrors DELETE /api/v1/apps/{name}/compose.
type ComposeDownResult struct {
	AppID           string   `json:"app_id"`
	Removed         []string `json:"removed"`
	TeardownPending []string `json:"teardown_pending,omitempty"`
	VolumesKept     []string `json:"volumes_kept,omitempty"`
	VolumesRemoved  []string `json:"volumes_removed,omitempty"`
	VolumeErrors    []string `json:"volume_errors,omitempty"`
}

// ComposePreview mirrors POST /api/v1/compose/preview.
type ComposePreview struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Services []struct {
		Key       string   `json:"key"`
		Image     string   `json:"image,omitempty"`
		Port      int      `json:"port,omitempty"`
		Domain    string   `json:"domain,omitempty"`
		Volumes   []string `json:"volumes,omitempty"`
		DependsOn []string `json:"depends_on,omitempty"`
		Env       []struct {
			Key       string `json:"key"`
			Generated bool   `json:"generated,omitempty"`
			Required  bool   `json:"required,omitempty"`
		} `json:"env,omitempty"`
	} `json:"services"`
	Profiles []string `json:"profiles,omitempty"`
	Skipped  []string `json:"skipped_services,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Notices  []struct {
		Level   string `json:"level"`
		Message string `json:"message"`
	} `json:"notices,omitempty"`
	NeedsRoot bool `json:"needs_root,omitempty"`
}

// TemplateInstallRequest is POST /api/v1/service-templates/{id}/install's body.
type TemplateInstallRequest struct {
	Name    string            `json:"name,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Domains map[string]string `json:"domains,omitempty"`
}

func profilesQuery(profiles []string) string {
	if len(profiles) == 0 {
		return ""
	}
	return "?profiles=" + url.QueryEscape(strings.Join(profiles, ","))
}

// postYAML sends a raw YAML body, which do() (JSON only) cannot.
func (c *Client) postYAML(ctx context.Context, path string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body)) //nolint:gosec // c.baseURL is the operator-supplied API target, same as do()
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "text/yaml")
	if c.token != "" {
		req.Header.Set(headerAuthorization, bearerPrefix+c.token)
	}
	start := time.Now()
	resp, err := c.hc.Do(req) //nolint:gosec // same target as do()
	if err != nil {
		TraceRequest(http.MethodPost, req.URL.String(), "", time.Since(start), err)
		return fmt.Errorf("request %s %s: %w", http.MethodPost, req.URL.String(), err)
	}
	defer func() { _ = resp.Body.Close() }()
	TraceRequest(http.MethodPost, req.URL.String(), resp.Status, time.Since(start), nil)
	return decodeResponse(resp, out)
}

// DeployComposeStack calls POST /api/v1/apps/{name}/compose with optional profiles.
func (c *Client) DeployComposeStack(ctx context.Context, name string, composeYAML []byte, profiles []string) (ComposeStackDeploy, error) {
	var out ComposeStackDeploy
	err := c.postYAML(ctx, "/api/v1/apps/"+PathEscape(name)+"/compose"+profilesQuery(profiles), composeYAML, &out)
	return out, err
}

// PreviewCompose calls POST /api/v1/compose/preview.
func (c *Client) PreviewCompose(ctx context.Context, composeYAML []byte, profiles []string) (ComposePreview, error) {
	var out ComposePreview
	err := c.postYAML(ctx, "/api/v1/compose/preview"+profilesQuery(profiles), composeYAML, &out)
	return out, err
}

// GetComposeStack calls GET /api/v1/apps/{name}/compose.
func (c *Client) GetComposeStack(ctx context.Context, name string) (ComposeStack, error) {
	var out ComposeStack
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/compose", nil, &out)
	return out, err
}

// ListComposeRevisions calls GET /api/v1/apps/{name}/compose/revisions.
func (c *Client) ListComposeRevisions(ctx context.Context, name string) ([]ComposeRevision, error) {
	var out struct {
		Revisions []ComposeRevision `json:"revisions"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/compose/revisions", nil, &out)
	return out.Revisions, err
}

// GetComposeRevision calls GET /api/v1/apps/{name}/compose/revisions/{revision}.
func (c *Client) GetComposeRevision(ctx context.Context, name string, revision int) (ComposeRevision, error) {
	var out ComposeRevision
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/compose/revisions/"+strconv.Itoa(revision), nil, &out)
	return out, err
}

// RollbackComposeStack calls POST /api/v1/apps/{name}/compose/rollback; revision 0 means the previous one.
func (c *Client) RollbackComposeStack(ctx context.Context, name string, revision int) (ComposeRollbackResult, error) {
	var out ComposeRollbackResult
	body := map[string]int{}
	if revision > 0 {
		body["revision"] = revision
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/compose/rollback", body, &out)
	return out, err
}

// RedeployComposeStack calls POST /api/v1/apps/{name}/compose/redeploy; service "" means every service.
func (c *Client) RedeployComposeStack(ctx context.Context, name, service string) ([]string, error) {
	var out struct {
		Restarted []string `json:"restarted"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/compose/redeploy", map[string]string{"service": service}, &out)
	return out.Restarted, err
}

// ComposeDown calls DELETE /api/v1/apps/{name}/compose.
func (c *Client) ComposeDown(ctx context.Context, name string, removeVolumes bool) (ComposeDownResult, error) {
	var out ComposeDownResult
	path := "/api/v1/apps/" + PathEscape(name) + "/compose"
	if removeVolumes {
		path += "?volumes=true"
	}
	err := c.do(ctx, http.MethodDelete, path, nil, &out)
	return out, err
}

// InstallTemplate calls POST /api/v1/service-templates/{id}/install.
func (c *Client) InstallTemplate(ctx context.Context, id string, req TemplateInstallRequest) (ComposeStackDeploy, error) {
	var out ComposeStackDeploy
	err := c.do(ctx, http.MethodPost, "/api/v1/service-templates/"+PathEscape(id)+"/install", req, &out)
	return out, err
}
