package apiclient

import (
	"context"
	"io"
	"net/http"
)

// VolumeImportResult counts what an import unpacked.
type VolumeImportResult struct {
	Entries int64 `json:"entries"`
	Bytes   int64 `json:"bytes"`
}

func volumeTransferPath(app, volume, verb string) string {
	return "/api/v1/apps/" + PathEscape(app) + "/volumes/" + PathEscape(volume) + "/" + verb
}

// ExportVolume streams an app volume as a tar.gz into w and returns the bytes written.
func (c *Client) ExportVolume(ctx context.Context, app, volume string, w io.Writer) (int64, error) {
	resp, err := c.streamRequest(ctx, http.MethodGet, volumeTransferPath(app, volume, "export"), nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, decodeResponse(resp, nil)
	}
	return io.Copy(w, resp.Body)
}

// ImportVolume uploads a tar.gz into a stopped app's volume; force replaces files already there.
func (c *Client) ImportVolume(ctx context.Context, app, volume string, r io.Reader, force bool) (VolumeImportResult, error) {
	p := volumeTransferPath(app, volume, "import")
	if force {
		p += "?force=true"
	}
	resp, err := c.streamRequest(ctx, http.MethodPost, p, r)
	if err != nil {
		return VolumeImportResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	var out VolumeImportResult
	return out, decodeResponse(resp, &out)
}
