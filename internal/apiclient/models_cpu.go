package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// ModelDownload mirrors internal/api's modelDownloadResource.
type ModelDownload struct {
	DoneBytes  int64 `json:"done_bytes"`
	TotalBytes int64 `json:"total_bytes"`
	Percent    int   `json:"percent"`
}

// ModelBenchPrompt mirrors internal/models.BenchPrompt.
type ModelBenchPrompt struct {
	Name             string  `json:"name"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TTFTMillis       int64   `json:"ttft_ms"`
	PromptTPS        float64 `json:"prompt_tokens_per_sec"`
	GenTPS           float64 `json:"gen_tokens_per_sec"`
}

// ModelBenchmarkResult mirrors internal/models.BenchResult.
type ModelBenchmarkResult struct {
	Model            string             `json:"model"`
	RanAt            time.Time          `json:"ran_at"`
	DurationMillis   int64              `json:"duration_ms"`
	Prompts          []ModelBenchPrompt `json:"prompts"`
	PromptTPS        float64            `json:"prompt_tokens_per_sec"`
	GenTPS           float64            `json:"gen_tokens_per_sec"`
	TTFTMillis       int64              `json:"ttft_ms"`
	PeakMemoryBytes  uint64             `json:"peak_memory_bytes"`
	MemoryLimitBytes uint64             `json:"memory_limit_bytes"`
	Truncated        bool               `json:"truncated"`
	Note             string             `json:"note"`
}

// CPUFit mirrors internal/models.CPUFit.
type CPUFit struct {
	Verdict        string   `json:"verdict"`
	Reason         string   `json:"reason"`
	Suggestion     string   `json:"suggestion,omitempty"`
	SuggestedID    string   `json:"suggested_id,omitempty"`
	CPUMilli       int      `json:"cpu_milli"`
	MemoryMiB      int      `json:"memory_mib"`
	ContextTokens  int      `json:"context_tokens"`
	WeightsBytes   int64    `json:"weights_bytes"`
	WeightsSource  string   `json:"weights_source"`
	KVBytes        int64    `json:"kv_bytes"`
	OverheadBytes  int64    `json:"overhead_bytes"`
	NeedBytes      int64    `json:"need_bytes"`
	CapBytes       int64    `json:"cap_bytes"`
	DiskBytes      int64    `json:"disk_bytes"`
	DiskOK         *bool    `json:"disk_ok"`
	TokensPerSecLo float64  `json:"tokens_per_sec_low"`
	TokensPerSecHi float64  `json:"tokens_per_sec_high"`
	Estimate       bool     `json:"estimate"`
	Arithmetic     string   `json:"arithmetic"`
	Warnings       []string `json:"warnings"`
	Note           string   `json:"note"`
}

// CPUBounds mirrors internal/models.CPUBounds.
type CPUBounds struct {
	DefaultCPUMilli  int   `json:"default_cpu_milli"`
	DefaultMemoryMiB int   `json:"default_memory_mib"`
	MaxCPUMilli      int   `json:"max_cpu_milli"`
	MaxMemoryMiB     int   `json:"max_memory_mib"`
	FreeRAMFloorMiB  int64 `json:"free_ram_floor_mib"`
}

// LocalModel mirrors internal/models.CatalogEntry.
type LocalModel struct {
	ID            string  `json:"id"`
	Label         string  `json:"label"`
	Vendor        string  `json:"vendor"`
	Repo          string  `json:"repo"`
	File          string  `json:"file"`
	ParamsB       float64 `json:"params_b"`
	Quant         string  `json:"quant"`
	DownloadBytes int64   `json:"download_bytes"`
	SHA256        string  `json:"sha256"`
	MinRAMMiB     int     `json:"min_ram_mib"`
	ContextLength int     `json:"context_length"`
	License       string  `json:"license"`
	Tier          string  `json:"tier"`
	Kind          string  `json:"kind"`
	ToolCalling   bool    `json:"tool_calling"`
	Notes         string  `json:"notes"`
}

// LocalModelCard is a curated entry with its estimate on a node.
type LocalModelCard struct {
	LocalModel
	Fit CPUFit `json:"fit"`
}

// LocalModelCatalog mirrors internal/models.CatalogView.
type LocalModelCatalog struct {
	Version    int              `json:"version"`
	VerifiedOn string           `json:"verified_on"`
	Note       string           `json:"note"`
	NodeID     string           `json:"node_id"`
	HostKnown  bool             `json:"host_known"`
	HostCPUs   int              `json:"host_cpus"`
	HostMemory int64            `json:"host_memory_bytes"`
	Bounds     CPUBounds        `json:"bounds"`
	Models     []LocalModelCard `json:"models"`
}

// CPUFitReport mirrors internal/models.CPUFitReport.
type CPUFitReport struct {
	CPUFit
	NodeID         string      `json:"node_id"`
	HostKnown      bool        `json:"host_known"`
	HostCPUs       int         `json:"host_cpus"`
	HostMemBytes   int64       `json:"host_memory_bytes"`
	HostMemFree    int64       `json:"host_memory_available_bytes"`
	DiskFreeBytes  *int64      `json:"disk_free_bytes"`
	Entry          *LocalModel `json:"entry,omitempty"`
	LicenseCurated bool        `json:"license_curated"`
	LicenseNotice  string      `json:"license_notice,omitempty"`
}

// GetLocalModelCatalog calls GET /api/v1/model-catalog.
func (c *Client) GetLocalModelCatalog(ctx context.Context, nodeID string) (LocalModelCatalog, error) {
	path := "/api/v1/model-catalog"
	if nodeID != "" {
		path += "?node_id=" + url.QueryEscape(nodeID)
	}
	var out LocalModelCatalog
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// CheckCPUModelFit calls POST /api/v1/models/fit in CPU mode.
func (c *Client) CheckCPUModelFit(ctx context.Context, req ModelFitRequest) (CPUFitReport, error) {
	req.Compute = "cpu"
	var out CPUFitReport
	err := c.do(ctx, http.MethodPost, "/api/v1/models/fit", req, &out)
	return out, err
}

// BenchmarkModel calls POST /api/v1/models/{name}/benchmark.
func (c *Client) BenchmarkModel(ctx context.Context, name string) (ModelBenchmarkResult, error) {
	var out ModelBenchmarkResult
	err := c.do(ctx, http.MethodPost, "/api/v1/models/"+PathEscape(name)+"/benchmark", nil, &out)
	return out, err
}

// ModelCPUFields mirrors internal/api's modelCPUResource. They live outside
// ModelResource so the MCP tool output schemas stay small.
type ModelCPUFields struct {
	Compute        string                `json:"compute,omitempty"`
	CPULimit       float64               `json:"cpu_limit,omitempty"`
	MemoryLimitMiB int                   `json:"memory_limit_mib,omitempty"`
	WeightsRepo    string                `json:"weights_repo,omitempty"`
	WeightsFile    string                `json:"weights_file,omitempty"`
	LicenseAck     bool                  `json:"license_acknowledged,omitempty"`
	Download       *ModelDownload        `json:"download,omitempty"`
	Benchmark      *ModelBenchmarkResult `json:"benchmark,omitempty"`
}

// ModelDetail is a ModelResource with its CPU mode fields.
type ModelDetail struct {
	ModelResource
	ModelCPUFields
}

// CreateModelDetailResponse is a ModelDetail plus the one-time API key.
type CreateModelDetailResponse struct {
	ModelDetail
	APIKey string `json:"api_key"`
}

// GetModelDetail calls GET /api/v1/models/{name} and keeps the CPU fields.
func (c *Client) GetModelDetail(ctx context.Context, name string) (ModelDetail, error) {
	var out ModelDetail
	err := c.do(ctx, http.MethodGet, "/api/v1/models/"+PathEscape(name), nil, &out)
	return out, err
}

// CreateModelDetail calls POST /api/v1/models and keeps the CPU fields.
func (c *Client) CreateModelDetail(ctx context.Context, req CreateModelRequest) (CreateModelDetailResponse, error) {
	var out CreateModelDetailResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/models", req, &out)
	return out, err
}
