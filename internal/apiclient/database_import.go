package apiclient

import (
	"context"
	"io"
	"net/http"
	"net/url"
)

// DatabaseImportCheck says whether a managed database holds data yet.
type DatabaseImportCheck struct {
	Database string `json:"database"`
	Engine   string `json:"engine"`
	Empty    bool   `json:"empty"`
}

// DatabaseImportResult is what a finished import reports; it never carries the dump.
type DatabaseImportResult struct {
	Database string `json:"database"`
	Engine   string `json:"engine"`
	Bytes    int64  `json:"bytes"`
	Replaced bool   `json:"replaced"`
}

func databaseImportPath(name string) string {
	return "/api/v1/databases/" + PathEscape(name) + "/import"
}

// CheckDatabaseImport calls GET /api/v1/databases/{name}/import.
func (c *Client) CheckDatabaseImport(ctx context.Context, name string) (DatabaseImportCheck, error) {
	var out DatabaseImportCheck
	err := c.do(ctx, http.MethodGet, databaseImportPath(name), nil, &out)
	return out, err
}

// ImportDatabase streams a dump to POST /api/v1/databases/{name}/import; force replaces existing data and needs the typed name as confirm.
func (c *Client) ImportDatabase(ctx context.Context, name string, dump io.Reader, force bool, confirm string) (DatabaseImportResult, error) {
	p := databaseImportPath(name)
	if force {
		p += "?" + url.Values{"force": {"true"}, "confirm": {confirm}}.Encode()
	}
	resp, err := c.streamRequest(ctx, http.MethodPost, p, dump)
	if err != nil {
		return DatabaseImportResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var out DatabaseImportResult
	return out, decodeResponse(resp, &out)
}
