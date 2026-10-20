package apiclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// SourceUpload mirrors internal/api's sourceUploadResource.
type SourceUpload struct {
	Hash           string `json:"hash"`
	Size           int64  `json:"size"`
	FileCount      int    `json:"file_count"`
	ExtractedBytes int64  `json:"extracted_bytes"`
	CreatedAt      string `json:"created_at"`
	ExpiresAt      string `json:"expires_at,omitempty"`
	Deduped        bool   `json:"deduped,omitempty"`
	// HasIndex and SiteRoot are set for zip uploads only.
	HasIndex *bool  `json:"has_index,omitempty"`
	SiteRoot string `json:"site_root,omitempty"`
}

// SourceUploadList is GET /api/v1/apps/{name}/uploads.
type SourceUploadList struct {
	Items      []SourceUpload `json:"items"`
	UsageBytes int64          `json:"usage_bytes"`
}

type progressReader struct {
	r    io.Reader
	sent int64
	fn   func(int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.sent += int64(n)
	if n > 0 && p.fn != nil {
		p.fn(p.sent)
	}
	return n, err
}

// UploadSource streams a gzip tar of size bytes to POST /api/v1/apps/{name}/uploads.
// It uses no overall timeout: a large archive on a slow link is bounded by ctx.
func (c *Client) UploadSource(ctx context.Context, name string, archive io.Reader, size int64, progress func(sent int64)) (SourceUpload, error) {
	return c.uploadArchive(ctx, name, archive, size, "application/gzip", progress)
}

// UploadSiteZip streams a zip of a static site to POST /api/v1/apps/{name}/uploads.
func (c *Client) UploadSiteZip(ctx context.Context, name string, archive io.Reader, size int64, progress func(sent int64)) (SourceUpload, error) {
	return c.uploadArchive(ctx, name, archive, size, "application/zip", progress)
}

func (c *Client) uploadArchive(ctx context.Context, name string, archive io.Reader, size int64, contentType string, progress func(sent int64)) (SourceUpload, error) {
	path := "/api/v1/apps/" + PathEscape(name) + "/uploads"
	body := &progressReader{r: archive, fn: progress}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, body) //nolint:gosec // c.baseURL is the operator-supplied API target
	if err != nil {
		return SourceUpload{}, fmt.Errorf("build request: %w", err)
	}
	req.ContentLength = size
	req.Header.Set(headerContentType, contentType)
	if c.token != "" {
		req.Header.Set(headerAuthorization, bearerPrefix+c.token)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	resp, err := (&http.Client{Transport: c.hc.Transport}).Do(req) //nolint:gosec // same target as above
	if err != nil {
		return SourceUpload{}, fmt.Errorf("request POST %s: %w", c.baseURL+path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out SourceUpload
	if err := decodeResponse(resp, &out); err != nil {
		return SourceUpload{}, err
	}
	return out, nil
}

// ListSourceUploads calls GET /api/v1/apps/{name}/uploads.
func (c *Client) ListSourceUploads(ctx context.Context, name string) (SourceUploadList, error) {
	var out SourceUploadList
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/uploads", nil, &out)
	return out, err
}
