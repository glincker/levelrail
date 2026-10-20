package backup

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeS3GetServer starts an httptest.Server standing in for an
// S3-compatible endpoint's GetObject call: enough of the REST API for a
// single GET to succeed or fail, the download-side counterpart of
// fakeS3Server (uploader_test.go). Not signature-validating, same
// reasoning that file's own doc comment gives.
func fakeS3GetServer(t *testing.T, status int, body string) (*httptest.Server, *recordedRequest) {
	t.Helper()
	rec := &recordedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.method = r.Method
		rec.host = r.Host
		rec.path = r.URL.Path
		rec.authz = r.Header.Get("Authorization")

		w.WriteHeader(status)
		if body != "" {
			_, _ = w.Write([]byte(body))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func TestS3Downloader_Download_Success(t *testing.T) {
	srv, rec := fakeS3GetServer(t, http.StatusOK, "pg_dump bytes coming back down")
	dest := testDestination(srv)

	var d S3Downloader
	rc, err := d.Download(context.Background(), dest, "mydb/mydb-20260814T030000Z.dump")
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	defer func() { _ = rc.Close() }()

	if rec.method != http.MethodGet {
		t.Errorf("method = %q, want %q", rec.method, http.MethodGet)
	}
	if rec.path != "/levelrail-backups/mydb/mydb-20260814T030000Z.dump" {
		t.Errorf("path = %q, want /levelrail-backups/mydb/mydb-20260814T030000Z.dump", rec.path)
	}

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != "pg_dump bytes coming back down" {
		t.Errorf("content = %q, want %q", got, "pg_dump bytes coming back down")
	}
}

// TestS3Downloader_Download_CustomEndpointIsUsed is
// TestS3Uploader_Upload_CustomEndpointIsUsed's exact download-side
// counterpart: proves the client dials dest.Endpoint with path-style
// addressing, not AWS's own default resolver, the same thing that
// matters for a restore pulling from an R2 or custom target as it does
// for a backup pushing to one.
func TestS3Downloader_Download_CustomEndpointIsUsed(t *testing.T) {
	srv, rec := fakeS3GetServer(t, http.StatusOK, "x")
	dest := testDestination(srv)

	var d S3Downloader
	rc, err := d.Download(context.Background(), dest, "key")
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	_ = rc.Close()

	wantHost := strings.TrimPrefix(srv.URL, "http://")
	if rec.host != wantHost {
		t.Errorf("request host = %q, want %q (the httptest server, not an AWS default endpoint)", rec.host, wantHost)
	}
	if !strings.HasPrefix(rec.path, "/levelrail-backups/") {
		t.Errorf("request path = %q, want it to start with /levelrail-backups/ (path-style addressing)", rec.path)
	}
}

// TestS3Downloader_Download_NotFoundSurfacesAsError proves a missing
// object becomes a real Go error, not a silent empty stream: the same
// "a rejection is a real error the caller can act on" contract
// TestS3Uploader_Upload_ServerErrorSurfacesAsError already establishes
// for the upload side, checked here against the specific failure mode a
// restore is most likely to hit in practice, an object_key that no
// longer exists in the bucket (deleted out of band, or a lifecycle rule
// expired it).
func TestS3Downloader_Download_NotFoundSurfacesAsError(t *testing.T) {
	const errBody = `<?xml version="1.0" encoding="UTF-8"?>
<Error><Code>NoSuchKey</Code><Message>test-injected not found</Message></Error>`
	srv, _ := fakeS3GetServer(t, http.StatusNotFound, errBody)
	dest := testDestination(srv)

	var d S3Downloader
	_, err := d.Download(context.Background(), dest, "mydb/does-not-exist.dump")
	if err == nil {
		t.Fatal("Download() error = nil, want the server's 404 surfaced")
	}
	if !strings.Contains(err.Error(), "backup: download") {
		t.Errorf("Download() error = %q, want it wrapped with this package's \"backup: download %%q\" context", err.Error())
	}
}
