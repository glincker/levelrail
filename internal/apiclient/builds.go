package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// BuildHint is the deterministic diagnosis attached to a failed build.
type BuildHint struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	Explanation string `json:"explanation"`
	Suggestion  string `json:"suggestion,omitempty"`
	Fix         string `json:"fix,omitempty"`
}

// BuildResource is one row of the build center.
type BuildResource struct {
	ID              string     `json:"id"`
	App             string     `json:"app"`
	Status          string     `json:"status"`
	Source          string     `json:"source"`
	Branch          string     `json:"branch,omitempty"`
	CommitSHA       string     `json:"commit_sha,omitempty"`
	Image           string     `json:"image,omitempty"`
	QueuedAt        *time.Time `json:"queued_at,omitempty"`
	StartedAt       time.Time  `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	DurationSeconds *float64   `json:"duration_seconds,omitempty"`
	QueuePosition   int        `json:"queue_position,omitempty"`
	CacheHitRatio   *float64   `json:"cache_hit_ratio,omitempty"`
	ImageBytes      int64      `json:"image_bytes,omitempty"`
	ImageDeltaBytes *int64     `json:"image_delta_bytes,omitempty"`
	Node            string     `json:"node,omitempty"`
	Platform        string     `json:"platform,omitempty"`
	Error           string     `json:"error,omitempty"`
	Hint            *BuildHint `json:"hint,omitempty"`
	LogsPath        string     `json:"logs_path"`
	CanCancel       bool       `json:"can_cancel"`
	CanRetry        bool       `json:"can_retry"`
}

// BuildCounts summarises the build center rows.
type BuildCounts struct {
	Queued   int `json:"queued"`
	Running  int `json:"running"`
	Finished int `json:"finished"`
	Failed   int `json:"failed"`
}

// BuildNodeLoad is one node's build slot usage.
type BuildNodeLoad struct {
	Node    string `json:"node"`
	Running int    `json:"running"`
	Waiting int    `json:"waiting"`
	Limit   int    `json:"limit"`
}

// BuildListResource is GET /api/v1/builds.
type BuildListResource struct {
	Rows      []BuildResource `json:"rows"`
	Counts    BuildCounts     `json:"counts"`
	Nodes     []BuildNodeLoad `json:"nodes"`
	Limit     int             `json:"max_concurrent_per_node"`
	Platforms struct {
		Host          string `json:"host"`
		MultiPlatform bool   `json:"multi_platform"`
	} `json:"platforms"`
}

// BuildCacheRecord is one BuildKit cache record.
type BuildCacheRecord struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Description string     `json:"description"`
	SizeBytes   int64      `json:"size_bytes"`
	InUse       bool       `json:"in_use"`
	UsageCount  int        `json:"usage_count"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

// BuildCacheResource is GET /api/v1/builds/cache.
type BuildCacheResource struct {
	Node             string             `json:"node"`
	Supported        bool               `json:"supported"`
	Reason           string             `json:"reason,omitempty"`
	TotalBytes       int64              `json:"total_bytes"`
	InUseBytes       int64              `json:"in_use_bytes"`
	ReclaimableBytes int64              `json:"reclaimable_bytes"`
	RecordCount      int                `json:"record_count"`
	Records          []BuildCacheRecord `json:"records"`
}

// BuildCachePruneRequest is the body of POST /api/v1/builds/cache/prune.
type BuildCachePruneRequest struct {
	Node           string `json:"node"`
	OlderThanHours *int   `json:"older_than_hours,omitempty"`
	Apply          bool   `json:"apply"`
}

// BuildCachePruneResource is the result of a prune preview or run.
type BuildCachePruneResource struct {
	Node           string             `json:"node"`
	Applied        bool               `json:"applied"`
	OlderThanHours int                `json:"older_than_hours"`
	Candidates     []BuildCacheRecord `json:"candidates"`
	CandidateCount int                `json:"candidate_count"`
	WouldFreeBytes int64              `json:"would_free_bytes"`
	FreedBytes     int64              `json:"freed_bytes"`
	RemovedCount   int                `json:"removed_count"`
}

// ListBuilds calls GET /api/v1/builds.
func (c *Client) ListBuilds(ctx context.Context, app, status string, limit int) (BuildListResource, error) {
	q := url.Values{}
	if app != "" {
		q.Set("app", app)
	}
	if status != "" {
		q.Set("status", status)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var out BuildListResource
	err := c.do(ctx, http.MethodGet, "/api/v1/builds?"+q.Encode(), nil, &out)
	return out, err
}

// GetBuild calls GET /api/v1/builds/{id}.
func (c *Client) GetBuild(ctx context.Context, id string) (BuildResource, error) {
	var out BuildResource
	err := c.do(ctx, http.MethodGet, "/api/v1/builds/"+url.PathEscape(id), nil, &out)
	return out, err
}

// CancelBuild calls POST /api/v1/apps/{app}/builds/{id}/cancel.
func (c *Client) CancelBuild(ctx context.Context, app, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/apps/"+url.PathEscape(app)+"/builds/"+url.PathEscape(id)+"/cancel", nil, nil)
}

// RetryBuild calls POST /api/v1/apps/{app}/builds/{id}/retry and returns the server's message.
func (c *Client) RetryBuild(ctx context.Context, app, id string) (string, error) {
	var out struct {
		Message string `json:"message"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+url.PathEscape(app)+"/builds/"+url.PathEscape(id)+"/retry", nil, &out)
	return out.Message, err
}

// GetBuildCache calls GET /api/v1/builds/cache.
func (c *Client) GetBuildCache(ctx context.Context, node string) (BuildCacheResource, error) {
	var out BuildCacheResource
	err := c.do(ctx, http.MethodGet, "/api/v1/builds/cache?"+url.Values{"node": {node}}.Encode(), nil, &out)
	return out, err
}

// PruneBuildCache calls POST /api/v1/builds/cache/prune.
func (c *Client) PruneBuildCache(ctx context.Context, req BuildCachePruneRequest) (BuildCachePruneResource, error) {
	var out BuildCachePruneResource
	err := c.do(ctx, http.MethodPost, "/api/v1/builds/cache/prune", req, &out)
	return out, err
}
