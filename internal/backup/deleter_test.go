package backup

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// recordedDeleteRequest mirrors uploader_test.go's recordedRequest,
// scoped to what these tests check about a DeleteObject call.
type recordedDeleteRequest struct {
	method string
	path   string
}

func fakeS3DeleteServer(t *testing.T, status int) (*httptest.Server, *recordedDeleteRequest) {
	t.Helper()
	rec := &recordedDeleteRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.method = r.Method
		rec.path = r.URL.Path
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

// TestS3Deleter_Delete_Success proves the basic path: a 204 response (the
// real S3-compatible answer to a successful DeleteObject) surfaces as no
// error, and the request hit the expected bucket/key path.
func TestS3Deleter_Delete_Success(t *testing.T) {
	srv, rec := fakeS3DeleteServer(t, http.StatusNoContent)
	dest := testDestination(srv)

	var d S3Deleter
	if err := d.Delete(context.Background(), dest, "mydb/mydb-1.dump"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if rec.method != http.MethodDelete {
		t.Errorf("method = %q, want %q", rec.method, http.MethodDelete)
	}
	if rec.path != "/levelrail-backups/mydb/mydb-1.dump" {
		t.Errorf("path = %q, want /levelrail-backups/mydb/mydb-1.dump", rec.path)
	}
}

// TestS3Deleter_Delete_AlreadyGoneIsNotAnError covers Deleter's own
// idempotency contract: real S3-compatible DeleteObject answers 204
// whether or not the key ever existed, so a key that's already been
// deleted (or never existed) must not surface as an error either, with
// no existence check needed in S3Deleter itself to make that true.
func TestS3Deleter_Delete_AlreadyGoneIsNotAnError(t *testing.T) {
	srv, _ := fakeS3DeleteServer(t, http.StatusNoContent)
	dest := testDestination(srv)

	var d S3Deleter
	if err := d.Delete(context.Background(), dest, "mydb/never-existed.dump"); err != nil {
		t.Fatalf("Delete() error = %v, want nil for a key that was already gone", err)
	}
}

// TestS3Deleter_Delete_ServerErrorSurfacesAsError proves a rejected
// delete becomes a real Go error, the same way S3Uploader's own
// server-error test proves it for uploads.
func TestS3Deleter_Delete_ServerErrorSurfacesAsError(t *testing.T) {
	srv, _ := fakeS3DeleteServer(t, http.StatusForbidden)
	dest := testDestination(srv)

	var d S3Deleter
	err := d.Delete(context.Background(), dest, "mydb/mydb-1.dump")
	if err == nil {
		t.Fatal("Delete() error = nil, want the server's rejection surfaced")
	}
	if !strings.Contains(err.Error(), "backup: delete") {
		t.Errorf("Delete() error = %q, want it wrapped with this package's \"backup: delete %%q from %%q\" context", err.Error())
	}
}
