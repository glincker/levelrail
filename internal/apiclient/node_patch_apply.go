package apiclient

import (
	"context"
	"net/http"
	"time"
)

// NodePatchPackage is one package a patch preview would change.
type NodePatchPackage struct {
	Name             string `json:"name"`
	CurrentVersion   string `json:"current_version,omitempty"`
	NewVersion       string `json:"new_version"`
	Source           string `json:"source,omitempty"`
	Security         bool   `json:"security"`
	Kernel           bool   `json:"kernel"`
	ContainerRuntime bool   `json:"container_runtime"`
}

// NodePatchPreview is POST /api/v1/nodes/{id}/patch/preview's response.
type NodePatchPreview struct {
	NodeID            string             `json:"node_id"`
	Manager           string             `json:"manager"`
	IncludeAll        bool               `json:"include_all"`
	Packages          []NodePatchPackage `json:"packages"`
	Held              []NodePatchPackage `json:"held"`
	OtherAvailable    int                `json:"other_available"`
	RebootRequiredNow bool               `json:"reboot_required_now"`
	RebootLikely      bool               `json:"reboot_likely"`
	PreviewID         string             `json:"preview_id"`
	ExpiresAt         time.Time          `json:"expires_at"`
}

// NodePatchApplyRequest is POST /api/v1/nodes/{id}/patch/apply's body.
type NodePatchApplyRequest struct {
	PreviewID        string `json:"preview_id"`
	IncludeAll       bool   `json:"include_all"`
	RebootIfRequired bool   `json:"reboot_if_required"`
	Confirm          bool   `json:"confirm"`
}

// NodePatchCounts is the patch status measured right after a run.
type NodePatchCounts struct {
	Total    int `json:"total"`
	Security int `json:"security"`
}

// NodePatchRun is one patch apply run.
type NodePatchRun struct {
	ID               string           `json:"id"`
	NodeID           string           `json:"node_id"`
	Actor            string           `json:"actor"`
	Manager          string           `json:"manager"`
	IncludeAll       bool             `json:"include_all"`
	RebootIfRequired bool             `json:"reboot_if_required"`
	Status           string           `json:"status"`
	StartedAt        time.Time        `json:"started_at"`
	FinishedAt       *time.Time       `json:"finished_at,omitempty"`
	Packages         []string         `json:"packages"`
	Held             []string         `json:"held"`
	ExitCode         int              `json:"exit_code"`
	Error            string           `json:"error,omitempty"`
	OutputTail       string           `json:"output_tail"`
	RebootRequired   bool             `json:"reboot_required"`
	RebootScheduled  bool             `json:"reboot_scheduled"`
	RebootNote       string           `json:"reboot_note,omitempty"`
	PatchStatusAfter *NodePatchCounts `json:"patch_status_after,omitempty"`
}

// PreviewNodePatch calls POST /api/v1/nodes/{id}/patch/preview.
func (c *Client) PreviewNodePatch(ctx context.Context, id string, includeAll, refresh bool) (NodePatchPreview, error) {
	var out NodePatchPreview
	body := map[string]bool{"include_all": includeAll, "refresh": refresh}
	err := c.do(ctx, http.MethodPost, nodePath(id)+"/patch/preview", body, &out)
	return out, err
}

// ApplyNodePatch calls POST /api/v1/nodes/{id}/patch/apply; the run continues in the background.
func (c *Client) ApplyNodePatch(ctx context.Context, id string, req NodePatchApplyRequest) (NodePatchRun, error) {
	var out NodePatchRun
	err := c.do(ctx, http.MethodPost, nodePath(id)+"/patch/apply", req, &out)
	return out, err
}

// ListNodePatchRuns calls GET /api/v1/nodes/{id}/patch/runs.
func (c *Client) ListNodePatchRuns(ctx context.Context, id string) ([]NodePatchRun, error) {
	var out []NodePatchRun
	err := c.do(ctx, http.MethodGet, nodePath(id)+"/patch/runs", nil, &out)
	return out, err
}
