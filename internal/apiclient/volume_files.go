package apiclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// VolumeFileEntry is one item in an app volume directory listing.
type VolumeFileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Type    string `json:"type"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
}

// VolumeFileListing is the response of GET .../volumes/{volume}/files.
type VolumeFileListing struct {
	Path      string            `json:"path"`
	Entries   []VolumeFileEntry `json:"entries"`
	Truncated bool              `json:"truncated"`
}

// VolumeFileWrite is the response of an upload or a single-file restore.
type VolumeFileWrite struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

func volumeFilesPath(app, volume string) string {
	return "/api/v1/apps/" + PathEscape(app) + "/volumes/" + PathEscape(volume) + "/files"
}

// ListVolumeFiles lists one directory of an app volume; limit 0 uses the server cap.
func (c *Client) ListVolumeFiles(ctx context.Context, app, volume, dir string, limit int) (VolumeFileListing, error) {
	q := url.Values{"path": {dir}}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var out VolumeFileListing
	err := c.do(ctx, http.MethodGet, volumeFilesPath(app, volume)+"?"+q.Encode(), nil, &out)
	return out, err
}

// DeleteVolumeFile removes one file from an app volume.
func (c *Client) DeleteVolumeFile(ctx context.Context, app, volume, p string) error {
	return c.do(ctx, http.MethodDelete, volumeFilesPath(app, volume)+"/content?"+url.Values{"path": {p}}.Encode(), nil, nil)
}

// RestoreVolumeFile copies one file out of a succeeded backup into the live volume.
func (c *Client) RestoreVolumeFile(ctx context.Context, app, volume, backupID, p string) (VolumeFileWrite, error) {
	var out VolumeFileWrite
	err := c.do(ctx, http.MethodPost, volumeFilesPath(app, volume)+"/restore", map[string]string{"backup_id": backupID, "path": p}, &out)
	return out, err
}

func (c *Client) streamRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body) //nolint:gosec // c.baseURL is the operator-supplied API target this client exists to call
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if c.token != "" {
		req.Header.Set(headerAuthorization, bearerPrefix+c.token)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	resp, err := c.hc.Do(req) //nolint:gosec // same target as above
	if err != nil {
		return nil, fmt.Errorf("request %s %s: %w", method, c.baseURL+path, err)
	}
	return resp, nil
}

// DownloadVolumeFile streams one volume file into w and returns the bytes written.
func (c *Client) DownloadVolumeFile(ctx context.Context, app, volume, p string, w io.Writer) (int64, error) {
	resp, err := c.streamRequest(ctx, http.MethodGet, volumeFilesPath(app, volume)+"/content?"+url.Values{"path": {p}}.Encode(), nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, decodeResponse(resp, nil)
	}
	return io.Copy(w, resp.Body)
}

// UploadVolumeFile streams r into a volume file, replacing any existing file at p.
func (c *Client) UploadVolumeFile(ctx context.Context, app, volume, p string, r io.Reader) (VolumeFileWrite, error) {
	resp, err := c.streamRequest(ctx, http.MethodPut, volumeFilesPath(app, volume)+"/content?"+url.Values{"path": {p}}.Encode(), r)
	if err != nil {
		return VolumeFileWrite{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var out VolumeFileWrite
	return out, decodeResponse(resp, &out)
}
