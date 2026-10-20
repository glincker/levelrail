package backup

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// recordedRequest captures the one HTTP request a fake S3 server in this
// file's tests expects to see: for a body small enough to fit in a
// single upload part (every payload in these tests), the AWS SDK's
// upload manager falls back to a single PutObject rather than a real
// multipart sequence, so one recorded request is the whole story. A
// pointer field, not a return value from the handler, because
// httptest.Server's handler runs on its own goroutine and the test body
// needs to read it back after ServeHTTP has already returned.
type recordedRequest struct {
	method string
	host   string
	path   string
	authz  string
	body   []byte
}

// fakeS3Server starts an httptest.Server standing in for an S3-compatible
// endpoint, just enough of the REST API for a single PutObject to
// succeed or fail: read the body, record what came in, and answer with
// status. It does not validate SigV4 signatures or implement multipart;
// neither is needed to prove S3Uploader sends the right bucket, key,
// body, and endpoint, which is what these tests are actually checking.
func fakeS3Server(t *testing.T, status int, respBody string) (*httptest.Server, *recordedRequest) {
	t.Helper()
	rec := &recordedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rec.method = r.Method
		rec.host = r.Host
		rec.path = r.URL.Path
		rec.authz = r.Header.Get("Authorization")
		rec.body = body

		w.Header().Set("ETag", `"deadbeefcafefeed"`)
		w.WriteHeader(status)
		if respBody != "" {
			_, _ = w.Write([]byte(respBody))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

// testDestination is a Destination pointed at srv, the shape every test
// below uses: an explicit Endpoint is exactly what a Custom or R2 target
// looks like in production (see Destination's own doc comment), so
// pointing it at a local httptest.Server rather than real R2 exercises
// the same code path a live R2 target would.
func testDestination(srv *httptest.Server) Destination {
	return Destination{
		Provider:        "custom",
		Endpoint:        srv.URL,
		Region:          "auto",
		Bucket:          "levelrail-backups",
		AccessKeyID:     "AKIATEST",
		SecretAccessKey: "shh",
	}
}

func TestS3Uploader_Upload_KnownSize(t *testing.T) {
	srv, rec := fakeS3Server(t, http.StatusOK, "")
	dest := testDestination(srv)
	body := "pg_dump bytes, known length up front"

	var u S3Uploader
	err := u.Upload(context.Background(), dest, "mydb/mydb-20260814T030000Z.dump", strings.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	if rec.method != http.MethodPut {
		t.Errorf("method = %q, want %q", rec.method, http.MethodPut)
	}
	if rec.path != "/levelrail-backups/mydb/mydb-20260814T030000Z.dump" {
		t.Errorf("path = %q, want /levelrail-backups/mydb/mydb-20260814T030000Z.dump", rec.path)
	}
	if string(rec.body) != body {
		t.Errorf("uploaded body = %q, want %q", rec.body, body)
	}
}

// TestS3Uploader_Upload_UnknownSize is the case Runner actually hits on
// every real backup (see runner.go's Upload call, size is always -1
// there): the body still arrives whole even with no declared length,
// because the upload manager reads r itself to find out where it ends
// rather than trusting the caller's size hint.
func TestS3Uploader_Upload_UnknownSize(t *testing.T) {
	srv, rec := fakeS3Server(t, http.StatusOK, "")
	dest := testDestination(srv)
	body := "streamed straight from a database dump that has not finished yet"

	var u S3Uploader
	err := u.Upload(context.Background(), dest, "mydb/dump.sql", strings.NewReader(body), -1)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if string(rec.body) != body {
		t.Errorf("uploaded body = %q, want %q", rec.body, body)
	}
}

// TestS3Uploader_Upload_CustomEndpointIsUsed proves the client actually
// dials dest.Endpoint rather than falling back to AWS's own per-region
// default resolver: the only network-observable difference between
// "custom endpoint wired up correctly" and "silently ignored, request
// went to AWS instead" is which host the request lands on, so that is
// what this test checks, plus path-style addressing (bucket in the
// path, not a subdomain), which R2 and every other non-AWS
// S3-compatible service in this codebase's supported providers needs.
func TestS3Uploader_Upload_CustomEndpointIsUsed(t *testing.T) {
	srv, rec := fakeS3Server(t, http.StatusOK, "")
	dest := testDestination(srv)

	var u S3Uploader
	if err := u.Upload(context.Background(), dest, "key", strings.NewReader("x"), 1); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	wantHost := strings.TrimPrefix(srv.URL, "http://")
	if rec.host != wantHost {
		t.Errorf("request host = %q, want %q (the httptest server, not an AWS default endpoint)", rec.host, wantHost)
	}
	if !strings.HasPrefix(rec.path, "/levelrail-backups/") {
		t.Errorf("request path = %q, want it to start with /levelrail-backups/ (path-style addressing)", rec.path)
	}
	if !strings.Contains(rec.authz, "/auto/s3/aws4_request") {
		t.Errorf("Authorization header = %q, want dest.Region (%q) in the SigV4 credential scope", rec.authz, dest.Region)
	}
}

// TestS3Uploader_Upload_ServerErrorSurfacesAsError proves a rejected
// upload becomes a real Go error Runner can record on the history row,
// not a silent success: the fake server here answers with the same
// shape a real S3-compatible service uses for a rejected request, an
// error status plus an XML error body.
func TestS3Uploader_Upload_ServerErrorSurfacesAsError(t *testing.T) {
	const errBody = `<?xml version="1.0" encoding="UTF-8"?>
<Error><Code>AccessDenied</Code><Message>test-injected access denied</Message></Error>`
	srv, _ := fakeS3Server(t, http.StatusForbidden, errBody)
	dest := testDestination(srv)

	var u S3Uploader
	err := u.Upload(context.Background(), dest, "key", strings.NewReader("x"), 1)
	if err == nil {
		t.Fatal("Upload() error = nil, want the server's rejection surfaced")
	}
	if !strings.Contains(err.Error(), "backup: upload to") {
		t.Errorf("Upload() error = %q, want it wrapped with this package's \"backup: upload to %%q\" context", err.Error())
	}
}
