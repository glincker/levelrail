package apiclient

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
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
	return copyVerifiedArchive(w, resp.Body)
}

// ErrExportTruncated means the export ended before a complete tar.gz arrived.
var ErrExportTruncated = errors.New("export truncated")

// copyVerifiedArchive copies a tar.gz to w while reading it through gzip and tar
// to the end, so a stream the server cut short fails instead of looking finished.
func copyVerifiedArchive(w io.Writer, body io.Reader) (int64, error) {
	pr, pw := io.Pipe()
	verdict := make(chan error, 1)
	go func() {
		err := verifyArchiveStream(pr)
		_, _ = io.Copy(io.Discard, pr) // never leave the writer blocked
		verdict <- err
	}()
	n, copyErr := io.Copy(io.MultiWriter(w, pw), body)
	_ = pw.Close()
	verr := <-verdict
	switch {
	case copyErr != nil:
		return n, fmt.Errorf("%w: %w", ErrExportTruncated, copyErr)
	case verr != nil:
		return n, fmt.Errorf("%w: %w", ErrExportTruncated, verr)
	}
	return n, nil
}

func verifyArchiveStream(r io.Reader) error {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	tr := tar.NewReader(zr)
	for {
		if _, err := tr.Next(); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return err
		}
	}
	// Reading past the tar end marker to the gzip trailer proves the stream was finished.
	_, err = io.Copy(io.Discard, zr) //nolint:gosec // output is discarded, only the stream end matters
	return err
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
