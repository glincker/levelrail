package backup

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Deleter is the real Deleter, covering AWS/R2/custom endpoints the
// same way S3Uploader does, reusing newS3Client (uploader.go) rather
// than building its own client.
type S3Deleter struct{}

// Delete removes key from dest.Bucket. A DeleteObject call against a key
// that is already gone (or never existed) returns success, not an
// error, the same way real S3-compatible services answer it: no
// existence check happens here on purpose, see Deleter's own doc
// comment.
func (S3Deleter) Delete(ctx context.Context, dest Destination, key string) error {
	client, err := newS3Client(ctx, dest)
	if err != nil {
		return fmt.Errorf("backup: delete %q from %q: %w", key, dest.Bucket, err)
	}

	if _, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(dest.Bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("backup: delete %q from %q: %w", key, dest.Bucket, err)
	}
	return nil
}
