package backup

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Tester probes a Destination for a lightweight, non-destructive
// connectivity and credential check, the test-connection counterpart
// Uploader/Deleter above establish for real backup/prune work.
type Tester interface {
	Test(ctx context.Context, dest Destination) error
}

// S3Tester is the real Tester, covering AWS/R2/custom endpoints the same
// way S3Uploader does, reusing newS3Client rather than building its own
// client.
type S3Tester struct{}

// Test issues a HeadBucket call against dest.Bucket: the cheapest S3
// operation that proves both the endpoint is reachable and the
// credentials are accepted, without reading or writing any object.
func (S3Tester) Test(ctx context.Context, dest Destination) error {
	client, err := newS3Client(ctx, dest)
	if err != nil {
		return fmt.Errorf("backup: test %q: %w", dest.Bucket, err)
	}

	if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(dest.Bucket),
	}); err != nil {
		return fmt.Errorf("backup: test %q: %w", dest.Bucket, err)
	}
	return nil
}
