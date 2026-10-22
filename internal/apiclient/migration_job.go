package apiclient

import (
	"context"
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/migrationgate"
)

// MigrationJobItem is one selected resource of a migration job.
type MigrationJobItem struct {
	SourceID       string `json:"source_id"`
	Name           string `json:"name"`
	Target         string `json:"target,omitempty"`
	State          string `json:"state"`
	Reason         string `json:"reason,omitempty"`
	Retryable      bool   `json:"retryable"`
	VolumesPending int    `json:"volumes_pending,omitempty"`
}

// MigrationJobDatabase is the data copy state of one database the job moves.
type MigrationJobDatabase struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Mismatched int    `json:"mismatched,omitempty"`
}

// MigrationJob is the compact, secret-free state of a migration job (an app import session).
type MigrationJob struct {
	ID        string                 `json:"id"`
	Platform  string                 `json:"platform"`
	SourceURL string                 `json:"source_url"`
	Step      string                 `json:"step"`
	Phase     string                 `json:"phase"`
	Running   bool                   `json:"running"`
	Connected bool                   `json:"connected"`
	Total     int                    `json:"total"`
	Done      int                    `json:"done"`
	Failed    int                    `json:"failed"`
	States    map[string]int         `json:"states"`
	Items     []MigrationJobItem     `json:"items"`
	Databases []MigrationJobDatabase `json:"databases"`
	Next      []string               `json:"next"`
	Revision  string                 `json:"revision"`
}

// MigrationSourceTest is the result of a source connection test.
type MigrationSourceTest struct {
	OK          bool     `json:"ok"`
	Platform    string   `json:"platform"`
	Failure     string   `json:"failure,omitempty"`
	Message     string   `json:"message,omitempty"`
	Apps        int      `json:"apps"`
	Databases   int      `json:"databases"`
	Unsupported int      `json:"unsupported"`
	Servers     []string `json:"servers"`
	Volumes     int      `json:"volumes"`
	Domains     int      `json:"domains"`
	Crons       int      `json:"crons"`
	Warnings    []string `json:"warnings"`
}

// MigrationProbeCheck is one DNS, certificate or health check of a domain.
type MigrationProbeCheck struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// MigrationProbe is the result of probing one domain of a job.
type MigrationProbe struct {
	Domain  string                `json:"domain"`
	App     string                `json:"app"`
	Targets []string              `json:"targets"`
	DNS     *MigrationProbeCheck  `json:"dns,omitempty"`
	HTTP    []MigrationProbeCheck `json:"http,omitempty"`
	OK      bool                  `json:"ok"`
}

// TestMigrationSource calls POST /api/v1/migration/apps/test. Nothing is stored.
func (c *Client) TestMigrationSource(ctx context.Context, req AppImportRequest) (MigrationSourceTest, error) {
	var out MigrationSourceTest
	err := c.do(ctx, http.MethodPost, appImportPath+"/test", req, &out)
	return out, err
}

// MigrationJob calls GET .../sessions/{id}/progress.
func (c *Client) MigrationJob(ctx context.Context, id string) (MigrationJob, error) {
	var out MigrationJob
	err := c.do(ctx, http.MethodGet, appImportPath+"/sessions/"+url.PathEscape(id)+"/progress", nil, &out)
	return out, err
}

// StageAppImportItems calls POST .../sessions/{id}/stage for the named items only, which is how one failed item is retried.
func (c *Client) StageAppImportItems(ctx context.Context, id string, items []string) (AppImportView, error) {
	var out AppImportView
	err := c.do(ctx, http.MethodPost, appImportPath+"/sessions/"+url.PathEscape(id)+"/stage", AppImportItemsRequest{Items: items}, &out)
	return out, err
}

// ProbeMigrationDomain calls POST .../sessions/{id}/probe. Kind is "dns", "http" or empty for both.
func (c *Client) ProbeMigrationDomain(ctx context.Context, id, domain, kind string) (MigrationProbe, error) {
	var out MigrationProbe
	body := map[string]string{"domain": domain, "kind": kind}
	err := c.do(ctx, http.MethodPost, appImportPath+"/sessions/"+url.PathEscape(id)+"/probe", body, &out)
	return out, err
}

// MigrationRetire reads the safe-to-delete checklist. With probe set it also runs the live DNS and response checks.
func (c *Client) MigrationRetire(ctx context.Context, id string, probe bool, acknowledged []string) (migrationgate.Report, error) {
	var out migrationgate.Report
	path := appImportPath + "/sessions/" + url.PathEscape(id) + "/retire"
	if !probe && len(acknowledged) == 0 {
		err := c.do(ctx, http.MethodGet, path, nil, &out)
		return out, err
	}
	err := c.do(ctx, http.MethodPost, path, map[string]any{"probe": probe, "acknowledged": acknowledged}, &out)
	return out, err
}

// ConnectAppImportSession calls POST .../sessions/{id}/connect: it re-attaches the source credential
// to an existing job after a restart. The credential stays in server memory and is never stored.
func (c *Client) ConnectAppImportSession(ctx context.Context, id string, req AppImportRequest) (AppImportView, error) {
	var out AppImportView
	err := c.do(ctx, http.MethodPost, appImportPath+"/sessions/"+url.PathEscape(id)+"/connect", req, &out)
	return out, err
}
