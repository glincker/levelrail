package backup

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestS3Tester_Test_Success proves a healthy bucket answers with no
// error, using the same fakeS3Server/testDestination helpers
// uploader_test.go already establishes for this package's S3-compatible
// tests.
func TestS3Tester_Test_Success(t *testing.T) {
	srv, rec := fakeS3Server(t, http.StatusOK, "")
	dest := testDestination(srv)

	var tester S3Tester
	if err := tester.Test(context.Background(), dest); err != nil {
		t.Fatalf("Test() error = %v", err)
	}
	if rec.method != http.MethodHead {
		t.Errorf("method = %q, want %q", rec.method, http.MethodHead)
	}
	if rec.path != "/levelrail-backups" {
		t.Errorf("path = %q, want /levelrail-backups", rec.path)
	}
}

// TestS3Tester_Test_Forbidden proves a credential the endpoint rejects
// surfaces as a real Go error, not a silent success: HeadBucket's own
// contract (see api_op_HeadBucket.go's doc comment) is a generic 403
// with no body for a bad credential.
func TestS3Tester_Test_Forbidden(t *testing.T) {
	srv, _ := fakeS3Server(t, http.StatusForbidden, "")
	dest := testDestination(srv)

	var tester S3Tester
	err := tester.Test(context.Background(), dest)
	if err == nil {
		t.Fatal("Test() error = nil, want the endpoint's rejection surfaced")
	}
	if !strings.Contains(err.Error(), "backup: test") {
		t.Errorf("Test() error = %q, want it wrapped with this package's \"backup: test %%q\" context", err.Error())
	}
}

// TestS3Tester_Test_BucketNotFound is HeadBucket's other documented
// generic status, a missing bucket, distinct from a rejected credential.
func TestS3Tester_Test_BucketNotFound(t *testing.T) {
	srv, _ := fakeS3Server(t, http.StatusNotFound, "")
	dest := testDestination(srv)

	var tester S3Tester
	if err := tester.Test(context.Background(), dest); err == nil {
		t.Fatal("Test() error = nil, want the missing bucket surfaced")
	}
}
