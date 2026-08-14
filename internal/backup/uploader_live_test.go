package backup

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestS3Uploader_Live_RoundTrip uploads a small real object to a real
// S3-compatible bucket, the same "skip cleanly without the real thing,
// never fail CI" pattern internal/docker and internal/reconcile's own
// _live_test.go files use for a reachable Docker daemon. Here the "real
// thing" is a bucket and credentials, supplied through env vars rather
// than autodetected, since there is no local daemon to probe:
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
}
