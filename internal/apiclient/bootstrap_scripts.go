package apiclient

import (
	"context"
	"net/http"
	"time"
)

// BootstrapScriptResource mirrors internal/api's bootstrapScriptResource; Body is empty in list results.
type BootstrapScriptResource struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Body      string    `json:"body,omitempty"`
	SizeBytes int       `json:"size_bytes"`
	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BootstrapScriptRequest is the create and update body.
type BootstrapScriptRequest struct {
	Name string `json:"name"`
	Body string `json:"body"`
}

// ListBootstrapScripts calls GET /api/v1/bootstrap-scripts.
func (c *Client) ListBootstrapScripts(ctx context.Context) ([]BootstrapScriptResource, error) {
	return getJSON[[]BootstrapScriptResource](ctx, c, "/api/v1/bootstrap-scripts")
}

// GetBootstrapScript calls GET /api/v1/bootstrap-scripts/{id}.
func (c *Client) GetBootstrapScript(ctx context.Context, id string) (BootstrapScriptResource, error) {
	return getJSON[BootstrapScriptResource](ctx, c, "/api/v1/bootstrap-scripts/"+PathEscape(id))
}

// CreateBootstrapScript calls POST /api/v1/bootstrap-scripts.
func (c *Client) CreateBootstrapScript(ctx context.Context, req BootstrapScriptRequest) (BootstrapScriptResource, error) {
	var out BootstrapScriptResource
	err := c.do(ctx, http.MethodPost, "/api/v1/bootstrap-scripts", req, &out)
	return out, err
}

// DeleteBootstrapScript calls DELETE /api/v1/bootstrap-scripts/{id}.
func (c *Client) DeleteBootstrapScript(ctx context.Context, id string) error {
	return c.del(ctx, "/api/v1/bootstrap-scripts/"+PathEscape(id))
}
