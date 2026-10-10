package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

const appImportPath = "/api/v1/migration/apps"

// AppImportMapping rewrites one source hostname to a new one in env values.
type AppImportMapping struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// AppImportRequest is the body of the plan and session create routes. Token
// is the source credential and is only ever sent in this body.
type AppImportRequest struct {
	Platform      string             `json:"platform"`
	URL           string             `json:"url"`
	Token         string             `json:"token"`
	InsecureTLS   bool               `json:"insecure_tls,omitempty"`
	AllowPrivate  bool               `json:"allow_private,omitempty"`
	AllowLoopback bool               `json:"allow_loopback,omitempty"`
	Only          []string           `json:"only,omitempty"`
	Collision     string             `json:"collision,omitempty"`
	Mappings      []AppImportMapping `json:"mappings,omitempty"`
}

// AppImportFinding is one reason behind a verdict.
type AppImportFinding struct {
	Reason string `json:"reason"`
	Next   string `json:"next,omitempty"`
}

// AppImportVolume is one persistent mount of a source app.
type AppImportVolume struct {
	Name          string `json:"name"`
	ContainerPath string `json:"container_path"`
	HostPath      string `json:"host_path,omitempty"`
	SizeBytes     int64  `json:"size_bytes"`
	SizeKnown     bool   `json:"size_known"`
}

// AppImportDatabase is a database an app points at.
type AppImportDatabase struct {
	SourceID   string `json:"source_id"`
	Name       string `json:"name"`
	Host       string `json:"host"`
	Target     string `json:"target,omitempty"`
	TargetHost string `json:"target_host,omitempty"`
}

// AppImportEntry is one inventory row. It never holds an env value.
type AppImportEntry struct {
	SourceID    string              `json:"source_id"`
	Name        string              `json:"name"`
	Kind        string              `json:"kind"`
	Project     string              `json:"project,omitempty"`
	Environment string              `json:"environment,omitempty"`
	Server      string              `json:"server,omitempty"`
	Source      string              `json:"source"`
	Repo        string              `json:"repo,omitempty"`
	Branch      string              `json:"branch,omitempty"`
	Image       string              `json:"image,omitempty"`
	BuildPack   string              `json:"build_pack,omitempty"`
	MapsTo      string              `json:"maps_to,omitempty"`
	Port        int                 `json:"port,omitempty"`
	Domains     []string            `json:"domains,omitempty"`
	Env         AppImportEnvCounts  `json:"env"`
	Volumes     []AppImportVolume   `json:"volumes,omitempty"`
	Databases   []AppImportDatabase `json:"databases,omitempty"`
	Verdict     string              `json:"verdict"`
	Findings    []AppImportFinding  `json:"findings,omitempty"`
}

// AppImportEnvCounts counts variables by sensitivity.
type AppImportEnvCounts struct {
	Plain  int `json:"plain"`
	Secret int `json:"secret"`
	Empty  int `json:"empty"`
}

// AppImportItem is one source app within a session.
type AppImportItem struct {
	SourceID  string         `json:"source_id"`
	Name      string         `json:"name"`
	Target    string         `json:"target,omitempty"`
	Kind      string         `json:"kind"`
	State     string         `json:"state"`
	Selected  bool           `json:"selected"`
	Reason    string         `json:"reason,omitempty"`
	Entry     AppImportEntry `json:"entry"`
	Domains   []string       `json:"domains,omitempty"`
	Remaining []string       `json:"remaining,omitempty"`
	AppPath   string         `json:"app_path,omitempty"`
}

// AppImportCheck is one preflight result.
type AppImportCheck struct {
	App    string `json:"app,omitempty"`
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// AppImportChange is one env value a mapping rewrote, with masked values.
type AppImportChange struct {
	App    string `json:"app"`
	Key    string `json:"key"`
	Secret bool   `json:"secret"`
	Count  int    `json:"count"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// AppImportPreflight is the outcome of the preflight checks.
type AppImportPreflight struct {
	Checks     []AppImportCheck  `json:"checks"`
	Diff       []AppImportChange `json:"diff"`
	Failed     int               `json:"failed"`
	Warnings   int               `json:"warnings"`
	EnvChecked bool              `json:"env_checked"`
	CanStage   bool              `json:"can_stage"`
}

// AppImportView is a plan or a session: the inventory, selection and state.
type AppImportView struct {
	ID        string              `json:"id"`
	Platform  string              `json:"platform"`
	SourceURL string              `json:"source_url"`
	Step      string              `json:"step"`
	Collision string              `json:"collision"`
	Mappings  []AppImportMapping  `json:"mappings"`
	Suggested []AppImportMapping  `json:"suggested_mappings"`
	Connected bool                `json:"connected"`
	Running   bool                `json:"running"`
	Items     []AppImportItem     `json:"items"`
	Databases []AppImportDatabase `json:"databases"`
	Preflight *AppImportPreflight `json:"preflight,omitempty"`
	Counts    map[string]int      `json:"counts"`
	States    map[string]int      `json:"states"`
}

// PlanAppImport calls POST /api/v1/migration/apps/plan. It stores nothing.
func (c *Client) PlanAppImport(ctx context.Context, req AppImportRequest) (AppImportView, error) {
	var out AppImportView
	err := c.do(ctx, http.MethodPost, appImportPath+"/plan", req, &out)
	return out, err
}

// CreateAppImportSession calls POST /api/v1/migration/apps/sessions.
func (c *Client) CreateAppImportSession(ctx context.Context, req AppImportRequest) (AppImportView, error) {
	var out AppImportView
	err := c.do(ctx, http.MethodPost, appImportPath+"/sessions", req, &out)
	return out, err
}

// GetAppImportSession calls GET /api/v1/migration/apps/sessions/{id}.
func (c *Client) GetAppImportSession(ctx context.Context, id string) (AppImportView, error) {
	var out AppImportView
	err := c.do(ctx, http.MethodGet, appImportPath+"/sessions/"+url.PathEscape(id), nil, &out)
	return out, err
}

// AppImportPlanUpdate is the body of PUT .../sessions/{id}/plan. Nil
// fields are left unchanged.
type AppImportPlanUpdate struct {
	Mappings  *[]AppImportMapping `json:"mappings,omitempty"`
	Selected  *[]string           `json:"selected,omitempty"`
	Collision string              `json:"collision,omitempty"`
}

// PutAppImportPlan calls PUT .../sessions/{id}/plan.
func (c *Client) PutAppImportPlan(ctx context.Context, id string, body AppImportPlanUpdate) (AppImportView, error) {
	var out AppImportView
	err := c.do(ctx, http.MethodPut, appImportPath+"/sessions/"+url.PathEscape(id)+"/plan", body, &out)
	return out, err
}

// AppImportItemsRequest narrows verify and rollback to some items.
type AppImportItemsRequest struct {
	Items []string `json:"items,omitempty"`
}

// StageAppImport calls POST .../sessions/{id}/stage. A blocked preflight
// is returned as an *APIError with the status 422.
func (c *Client) StageAppImport(ctx context.Context, id string) (AppImportView, error) {
	var out AppImportView
	err := c.do(ctx, http.MethodPost, appImportPath+"/sessions/"+url.PathEscape(id)+"/stage", map[string]any{}, &out)
	return out, err
}

// VerifyAppImport calls POST .../sessions/{id}/verify.
func (c *Client) VerifyAppImport(ctx context.Context, id string, items []string) (AppImportView, error) {
	var out AppImportView
	err := c.do(ctx, http.MethodPost, appImportPath+"/sessions/"+url.PathEscape(id)+"/verify", AppImportItemsRequest{Items: items}, &out)
	return out, err
}

// RollbackAppImport calls POST .../sessions/{id}/rollback.
func (c *Client) RollbackAppImport(ctx context.Context, id string, items []string) (AppImportView, error) {
	var out AppImportView
	err := c.do(ctx, http.MethodPost, appImportPath+"/sessions/"+url.PathEscape(id)+"/rollback", AppImportItemsRequest{Items: items}, &out)
	return out, err
}

// AppImportReceipt calls GET .../sessions/{id}/receipt and returns the JSON as is.
func (c *Client) AppImportReceipt(ctx context.Context, id string) (json.RawMessage, error) {
	var out json.RawMessage
	err := c.do(ctx, http.MethodGet, appImportPath+"/sessions/"+url.PathEscape(id)+"/receipt", nil, &out)
	return out, err
}
