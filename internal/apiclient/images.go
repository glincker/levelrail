package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ImageAppUse says how an app relates to one image.
type ImageAppUse struct {
	App       string    `json:"app"`
	Role      string    `json:"role"`
	DeployID  string    `json:"deploy_id,omitempty"`
	Source    string    `json:"source,omitempty"`
	CommitSHA string    `json:"commit_sha,omitempty"`
	Branch    string    `json:"branch,omitempty"`
	At        time.Time `json:"at,omitempty"`
}

// ImageEntry is one image on a node with its owners and retention status.
type ImageEntry struct {
	ID            string        `json:"id"`
	Digest        string        `json:"digest,omitempty"`
	Tags          []string      `json:"tags"`
	SizeBytes     int64         `json:"size_bytes"`
	SharedBytes   int64         `json:"shared_bytes"`
	Containers    int64         `json:"containers"`
	CreatedAt     time.Time     `json:"created_at"`
	Apps          []ImageAppUse `json:"apps"`
	Protected     bool          `json:"protected"`
	ProtectReason string        `json:"protect_reason,omitempty"`
}

// ImageRetention is the retention policy in effect.
type ImageRetention struct {
	KeepPerApp int `json:"keep_per_app"`
	MinAgeHrs  int `json:"min_age_hours"`
}

// ImageTotals summarizes an inventory.
type ImageTotals struct {
	Count          int   `json:"count"`
	SizeBytes      int64 `json:"size_bytes"`
	ReclaimBytes   int64 `json:"reclaimable_bytes"`
	ProtectedCount int   `json:"protected_count"`
}

// ImageSignatureStatus is the signature verification setup.
type ImageSignatureStatus struct {
	Mode       string `json:"mode"`
	Configured bool   `json:"configured"`
}

// ImageListResource is GET /api/v1/images.
type ImageListResource struct {
	Node       string               `json:"node"`
	Images     []ImageEntry         `json:"images"`
	Totals     ImageTotals          `json:"totals"`
	Retention  ImageRetention       `json:"retention"`
	Signatures ImageSignatureStatus `json:"signatures"`
}

// ImageLayer is one layer of an image.
type ImageLayer struct {
	ID        string    `json:"id"`
	CreatedBy string    `json:"created_by"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

// ImageLayersResource is GET /api/v1/images/layers.
type ImageLayersResource struct {
	Layers    []ImageLayer `json:"layers"`
	SizeBytes int64        `json:"size_bytes"`
}

// ImageGCRequest is the body of POST /api/v1/images/gc.
type ImageGCRequest struct {
	Node       string `json:"node,omitempty"`
	KeepPerApp *int   `json:"keep_per_app,omitempty"`
	MinAgeHrs  *int   `json:"min_age_hours,omitempty"`
	DryRun     bool   `json:"dry_run"`
}

// ImageGCFailure is one image retention could not remove.
type ImageGCFailure struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

// ImageGCResource is the result of POST /api/v1/images/gc.
type ImageGCResource struct {
	DryRun         bool             `json:"dry_run"`
	Candidates     []ImageEntry     `json:"candidates"`
	Removed        []string         `json:"removed"`
	Failed         []ImageGCFailure `json:"failed"`
	ReclaimedBytes int64            `json:"reclaimed_bytes"`
	Retention      ImageRetention   `json:"retention"`
}

// ImageVerifyRequest is the body of POST /api/v1/images/verify.
type ImageVerifyRequest struct {
	Node string `json:"node,omitempty"`
	ID   string `json:"id,omitempty"`
	Ref  string `json:"ref,omitempty"`
}

// ImageVerifyResource is the result of a signature check.
type ImageVerifyResource struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// ListImages calls GET /api/v1/images. keepPerApp < 0 keeps the server default.
func (c *Client) ListImages(ctx context.Context, node string, keepPerApp int) (ImageListResource, error) {
	q := url.Values{}
	if node != "" {
		q.Set("node", node)
	}
	if keepPerApp >= 0 {
		q.Set("keep_per_app", strconv.Itoa(keepPerApp))
	}
	var out ImageListResource
	err := c.do(ctx, http.MethodGet, "/api/v1/images?"+q.Encode(), nil, &out)
	return out, err
}

// GetImageLayers calls GET /api/v1/images/layers.
func (c *Client) GetImageLayers(ctx context.Context, node, id string) (ImageLayersResource, error) {
	q := url.Values{"id": {id}}
	if node != "" {
		q.Set("node", node)
	}
	var out ImageLayersResource
	err := c.do(ctx, http.MethodGet, "/api/v1/images/layers?"+q.Encode(), nil, &out)
	return out, err
}

// RunImageGC calls POST /api/v1/images/gc.
func (c *Client) RunImageGC(ctx context.Context, req ImageGCRequest) (ImageGCResource, error) {
	var out ImageGCResource
	err := c.do(ctx, http.MethodPost, "/api/v1/images/gc", req, &out)
	return out, err
}

// VerifyImage calls POST /api/v1/images/verify.
func (c *Client) VerifyImage(ctx context.Context, req ImageVerifyRequest) (ImageVerifyResource, error) {
	var out ImageVerifyResource
	err := c.do(ctx, http.MethodPost, "/api/v1/images/verify", req, &out)
	return out, err
}
