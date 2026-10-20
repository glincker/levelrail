package backup

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Uploader is the real Uploader: every backup target this codebase
// supports (AWS, R2, and any other S3-compatible custom endpoint) speaks
// the same REST API, so one implementation covers all three, driven
// entirely by the Provider/Endpoint distinction already captured on
// Destination.
//
// A new S3 client is built per Upload call rather than cached on the
// struct: Destination carries live, per-target credentials resolved
// fresh from internal/secrets on every backup run (see Runner), and a
// target's bucket, region, or endpoint can change between runs too.
// Backups are infrequent (the default schedule is once a day, see
// store.BackupTarget's own docs), so the cost of rebuilding a client
// each time is not worth the complexity of a cache that would need
// invalidating whenever a target's settings change.
type S3Uploader struct{}

// Upload writes r to dest.Bucket under key using the AWS SDK's S3
// upload manager rather than a raw PutObject call, because size is
// frequently -1 here: Runner streams a database dump straight from a
// running pg_dump/mysqldump process into Upload while the dump is still
// in flight, so there is no Content-Length to give PutObject and no
// seekable body to compute one from. The upload manager handles this by
// buffering r into fixed-size parts and issuing an S3 multipart upload,
// which needs no upfront length. The size parameter goes unused (named
// _ below, the same convention this package's own test fakes already
// use for it): it is a hint for implementations that can act on it (see
// backup.go's own doc comment on Uploader), and this one cannot, since
// the manager sizes itself from r as it streams regardless of what is
// passed.
//
// This intentionally uses feature/s3/manager rather than its
// replacement, feature/s3/transfermanager: as of this writing
// transfermanager's UploadObject takes a Body that must support Len()
// or Seek() to size itself, which a streaming pg_dump/mysqldump pipe
// cannot provide, and its own maintainers describe the unknown-size
// streaming case as unsupported. feature/s3/manager is marked
// deprecated in its doc comments but not removed or broken; it is the
// only one of the two that actually does what Runner needs. Revisit
// this once transfermanager grows real support for an unbounded
// io.Reader body.
func (S3Uploader) Upload(ctx context.Context, dest Destination, key string, r io.Reader, _ int64) error {
	client, err := newS3Client(ctx, dest)
	if err != nil {
		return fmt.Errorf("backup: upload to %q: %w", dest.Bucket, err)
	}

	uploader := manager.NewUploader(client) //nolint:staticcheck // feature/s3/manager is deprecated in favor of transfermanager, which does not yet support an unbounded io.Reader body; see this method's doc comment

	input := &s3.PutObjectInput{
		Bucket: aws.String(dest.Bucket),
		Key:    aws.String(key),
		Body:   r,
	}

	if _, err := uploader.Upload(ctx, input); err != nil { //nolint:staticcheck // same deprecation as NewUploader above
		return fmt.Errorf("backup: upload to %q: %w", dest.Bucket, err)
	}
	return nil
}

// newS3Client builds an S3 client scoped to a single Destination:
// static credentials from dest (never the ambient environment or an
// instance role, since these are a specific target's own keys, not
// this process's own AWS identity) and, when dest.Endpoint is set, that
// endpoint in place of AWS's own per-region resolver.
//
// dest.Provider is deliberately not consulted here for anything beyond
// what dest.Endpoint already encodes: Destination's own doc comment
// establishes Endpoint as the final authority whenever it is non-empty,
// so branching on Provider as well would just create two sources of
// truth that could disagree. Path-style addressing is instead keyed off
// "is a custom endpoint set at all", which is the actual reason it is
// needed: R2, MinIO, Backblaze B2, and every other non-AWS S3-compatible
// service in practice expects <endpoint>/<bucket>/<key> rather than
// AWS's <bucket>.<endpoint>/<key> virtual-hosted form, and AWS itself
// (empty Endpoint) is untouched by this and keeps its default
// virtual-hosted resolution.
func newS3Client(ctx context.Context, dest Destination) (*s3.Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(dest.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			dest.AccessKeyID, dest.SecretAccessKey, "",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("load client config: %w", err)
	}

	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		if dest.Endpoint != "" {
			o.BaseEndpoint = aws.String(dest.Endpoint)
			o.UsePathStyle = true
		}
	}), nil
}
