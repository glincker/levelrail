package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// TestS3Uploader_Live_RoundTrip uploads a small real object to a real
// S3-compatible bucket and then downloads it back through S3Downloader,
// checking the bytes that come back match what went up: a genuine round
// trip, not just an upload with nothing verifying the object actually
// landed the way a later restore would need it to. Added when
// S3Downloader was introduced (internal/backup's restore path): before
// that, this test's name overstated what it checked, upload only, with
// no download side to close the loop.
//
// Same "skip cleanly without the real thing, never fail CI" pattern
// internal/docker and internal/reconcile's own _live_test.go files use
// for a reachable Docker daemon. Here the "real thing" is a bucket and
// credentials, supplied through env vars rather than autodetected, since
// there is no local daemon to probe:
//
//   - LEVELRAIL_LIVE_S3_ENDPOINT: the S3-compatible API base URL (an R2
//     account endpoint, a MinIO instance, or empty for real AWS S3).
//   - LEVELRAIL_LIVE_S3_REGION: required.
//   - LEVELRAIL_LIVE_S3_BUCKET: required, must already exist.
//   - LEVELRAIL_LIVE_S3_ACCESS_KEY_ID / LEVELRAIL_LIVE_S3_SECRET_ACCESS_KEY: required.
//
// None of these are set in CI today, so this test skips there; it exists
// for a developer to run by hand against their own R2 or S3 bucket
// before trusting a change to this file.
func TestS3Uploader_Live_RoundTrip(t *testing.T) {
	bucket := os.Getenv("LEVELRAIL_LIVE_S3_BUCKET")
	accessKeyID := os.Getenv("LEVELRAIL_LIVE_S3_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("LEVELRAIL_LIVE_S3_SECRET_ACCESS_KEY")
	region := os.Getenv("LEVELRAIL_LIVE_S3_REGION")
	if bucket == "" || accessKeyID == "" || secretAccessKey == "" || region == "" {
		t.Skipf("LEVELRAIL_LIVE_S3_BUCKET, _ACCESS_KEY_ID, _SECRET_ACCESS_KEY and _REGION not all set, skipping live upload test")
	}

	dest := Destination{
		Provider:        "custom",
		Endpoint:        os.Getenv("LEVELRAIL_LIVE_S3_ENDPOINT"),
		Region:          region,
		Bucket:          bucket,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
	}
	key := fmt.Sprintf("levelrail-live-test/%d.txt", time.Now().UnixNano())
	body := "levelrail internal/backup live upload test"

	var u S3Uploader
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := u.Upload(ctx, dest, key, strings.NewReader(body), int64(len(body))); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	var d S3Downloader
	rc, err := d.Download(ctx, dest, key)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	defer func() { _ = rc.Close() }()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("reading Download() stream error = %v", err)
	}
	if string(got) != body {
		t.Errorf("downloaded content = %q, want %q (the exact bytes just uploaded)", got, body)
	}
}
