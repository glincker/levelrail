package backup

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Downloader retrieves a single object from a Destination, the exact
// reverse of Uploader: given the same (Destination, key) an earlier
// Upload call was made with, it returns that object's bytes as a stream.
// Implementations are not required to know the object's size up front
// (see S3Downloader's own doc comment for why GetObject already makes
// this a non-issue, unlike Upload's own unknown-size handling), only to
// stream it back faithfully.
type Downloader interface {
	Download(ctx context.Context, dest Destination, key string) (io.ReadCloser, error)
}

// S3Downloader is the real Downloader: the download side of the same
// AWS/R2/custom-endpoint story S3Uploader already covers, one
// implementation for every provider this codebase supports, driven by
// the same Destination.Endpoint-is-the-final-authority rule.
//
// Unlike S3Uploader, this has no multipart-vs-single-PUT decision to
// make: S3's GetObject API already streams a response body of whatever
// size the object actually is, with no equivalent to PutObject's
// Content-Length requirement on the request side, so there's no
// unknown-size problem here to work around with an upload-manager-style
// abstraction. A plain client.GetObject call is the entire
// implementation.
type S3Downloader struct{}

// Download implements Downloader. The returned ReadCloser is
// GetObjectOutput.Body itself, not copied into a buffer first: a
// database dump can be large, and RunRestore already has to stream it
// straight into ContainerRestorer.Restore's stdin (see restore.go), so
// buffering the whole object in this process's memory first would cost
// real memory for no benefit; Close on the returned value releases the
// underlying HTTP response exactly the same way Dumper's own
// ReadCloser contract already requires callers to honor.
func (S3Downloader) Download(ctx context.Context, dest Destination, key string) (io.ReadCloser, error) {
	client, err := newS3Client(ctx, dest)
	if err != nil {
		return nil, fmt.Errorf("backup: download %q from %q: %w", key, dest.Bucket, err)
	}

	out, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(dest.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("backup: download %q from %q: %w", key, dest.Bucket, err)
	}
	return out.Body, nil
}
