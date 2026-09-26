package apiclient

import (
	"context"
	"net/http"
	"time"
)

// ModelPreflightRequest mirrors internal/api's modelPreflightRequest.
type ModelPreflightRequest struct {
	Repo    string `json:"repo"`
	Engine  string `json:"engine,omitempty"`
	File    string `json:"file,omitempty"`
	Quant   string `json:"quant,omitempty"`
	NodeID  string `json:"node_id,omitempty"`
	HFToken string `json:"hf_token,omitempty"`
}

// PreflightFile mirrors internal/models' PreflightFile.
type PreflightFile struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// PreflightQuant mirrors internal/models' Quant.
type PreflightQuant struct {
	Name        string   `json:"name"`
	Bytes       int64    `json:"bytes"`
	Files       []string `json:"files"`
	Fit         string   `json:"fit"`
	Recommended bool     `json:"recommended"`
}

// PreflightSelection mirrors internal/models' PreflightSelection.
type PreflightSelection struct {
	Label string `json:"label"`
	Bytes int64  `json:"bytes"`
	Fit   string `json:"fit"`
}

// PreflightNode mirrors internal/models' PreflightNode. Nil is unknown.
type PreflightNode struct {
	NodeID         string `json:"node_id"`
	GPUPresent     bool   `json:"gpu_present"`
	VRAMTotalBytes *int64 `json:"vram_total_bytes"`
	VRAMFreeBytes  *int64 `json:"vram_free_bytes"`
	DiskFreeBytes  *int64 `json:"disk_free_bytes"`
	DiskTotalBytes *int64 `json:"disk_total_bytes"`
}

// PreflightDisk mirrors internal/models' PreflightDisk.
type PreflightDisk struct {
	Status        string `json:"status"`
	RequiredBytes int64  `json:"required_bytes"`
	FreeBytes     *int64 `json:"free_bytes"`
	Message       string `json:"message"`
}

// ModelPreflightResult mirrors internal/models' PreflightResult.
type ModelPreflightResult struct {
	Repo               string              `json:"repo"`
	Status             string              `json:"status"`
	Exists             bool                `json:"exists"`
	Message            string              `json:"message"`
	NextStep           string              `json:"next_step,omitempty"`
	RetryAfterSeconds  int                 `json:"retry_after_seconds,omitempty"`
	Gated              string              `json:"gated,omitempty"`
	Access             string              `json:"access,omitempty"`
	Private            bool                `json:"private"`
	License            string              `json:"license,omitempty"`
	TotalBytes         int64               `json:"total_bytes"`
	FileCount          int                 `json:"file_count"`
	Files              []PreflightFile     `json:"files"`
	FilesTruncated     bool                `json:"files_truncated"`
	HasGGUF            bool                `json:"has_gguf"`
	HasSafetensors     bool                `json:"has_safetensors"`
	CompatibleEngines  []string            `json:"compatible_engines"`
	EngineHint         string              `json:"engine_hint"`
	Quants             []PreflightQuant    `json:"quants"`
	RecommendedQuant   string              `json:"recommended_quant,omitempty"`
	RecommendationNote string              `json:"recommendation_note,omitempty"`
	Selected           *PreflightSelection `json:"selected,omitempty"`
	Node               PreflightNode       `json:"node"`
	Disk               PreflightDisk       `json:"disk"`
	Warnings           []string            `json:"warnings"`
	EstimateNote       string              `json:"estimate_note"`
	Cached             bool                `json:"cached"`
}

// ModelCacheEntry mirrors internal/models' CacheEntry.
type ModelCacheEntry struct {
	Volume         string     `json:"volume"`
	Model          string     `json:"model,omitempty"`
	Engine         string     `json:"engine,omitempty"`
	ModelRef       string     `json:"model_ref,omitempty"`
	SizeBytes      *int64     `json:"size_bytes"`
	LastUsedAt     *time.Time `json:"last_used_at"`
	LastUsedSource string     `json:"last_used_source"`
	UnusedDays     int        `json:"unused_days"`
	Unused         bool       `json:"unused"`
	InUse          bool       `json:"in_use"`
	Configured     bool       `json:"configured"`
	DuplicateOf    string     `json:"duplicate_of,omitempty"`
	Prunable       bool       `json:"prunable"`
	KeepReason     string     `json:"keep_reason,omitempty"`
}

// ModelCacheNode mirrors internal/models' CacheNode.
type ModelCacheNode struct {
	NodeID           string            `json:"node_id"`
	Name             string            `json:"name"`
	IsLocal          bool              `json:"is_local"`
	Supported        bool              `json:"supported"`
	Message          string            `json:"message,omitempty"`
	Entries          []ModelCacheEntry `json:"entries"`
	TotalBytes       int64             `json:"total_bytes"`
	UniqueBytes      int64             `json:"unique_bytes"`
	ReclaimableBytes int64             `json:"reclaimable_bytes"`
	DiskFreeBytes    *int64            `json:"disk_free_bytes"`
}

// ModelCacheReport mirrors internal/models' CacheReport.
type ModelCacheReport struct {
	UnusedDays int              `json:"unused_days"`
	Nodes      []ModelCacheNode `json:"nodes"`
	Note       string           `json:"note"`
}

// ModelCacheSkip mirrors internal/models' CacheSkip.
type ModelCacheSkip struct {
	Volume string `json:"volume"`
	Reason string `json:"reason"`
}

// ModelCachePruneResult mirrors internal/models' CachePruneResult.
type ModelCachePruneResult struct {
	DryRun         bool              `json:"dry_run"`
	Candidates     []ModelCacheEntry `json:"candidates"`
	Removed        []string          `json:"removed"`
	Skipped        []ModelCacheSkip  `json:"skipped"`
	ReclaimedBytes int64             `json:"reclaimed_bytes"`
}

type modelCachePruneRequest struct {
	Volumes []string `json:"volumes,omitempty"`
	DryRun  bool     `json:"dry_run"`
}

// PreflightModel calls POST /api/v1/models/preflight.
func (c *Client) PreflightModel(ctx context.Context, req ModelPreflightRequest) (ModelPreflightResult, error) {
	var out ModelPreflightResult
	err := c.do(ctx, http.MethodPost, "/api/v1/models/preflight", req, &out)
	return out, err
}

// ListModelCache calls GET /api/v1/model-cache.
func (c *Client) ListModelCache(ctx context.Context) (ModelCacheReport, error) {
	var out ModelCacheReport
	err := c.do(ctx, http.MethodGet, "/api/v1/model-cache", nil, &out)
	return out, err
}

// PruneModelCache calls POST /api/v1/model-cache/prune.
func (c *Client) PruneModelCache(ctx context.Context, volumes []string, dryRun bool) (ModelCachePruneResult, error) {
	var out ModelCachePruneResult
	err := c.do(ctx, http.MethodPost, "/api/v1/model-cache/prune", modelCachePruneRequest{Volumes: volumes, DryRun: dryRun}, &out)
	return out, err
}
