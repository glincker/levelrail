package mailstorage

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

// CreateYourselfError means the key may not create the bucket, so the
// operator must; Policy is the exact least-privilege text to apply.
type CreateYourselfError struct {
	Provider string
	Policy   string
	Guidance string
}

func (e *CreateYourselfError) Error() string {
	return "create the bucket yourself: " + e.Guidance
}

// IsR2 reports whether the endpoint is Cloudflare R2.
func IsR2(endpoint string) bool {
	return strings.Contains(endpoint, ".r2.cloudflarestorage.com")
}

// LeastPrivilegePolicy is the S3 policy limited to one service prefix.
func LeastPrivilegePolicy(bucket, serviceID string) string {
	return fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:PutObject", "s3:GetObject", "s3:DeleteObject"],
      "Resource": "arn:aws:s3:::%[1]s/%[2]s/*"
    },
    {
      "Effect": "Allow",
      "Action": "s3:ListBucket",
      "Resource": "arn:aws:s3:::%[1]s",
      "Condition": { "StringLike": { "s3:prefix": ["%[2]s/*"] } }
    }
  ]
}`, bucket, serviceID)
}

// R2Guidance explains the one-bucket-per-service rule, since R2 tokens
// scope to buckets and not to prefixes.
func R2Guidance(bucket string) string {
	return fmt.Sprintf("Cloudflare R2 tokens scope to a bucket, not a prefix. Create a bucket named %q for this mail service only, then issue an Object Read and Write token scoped to that one bucket.", bucket)
}

func createYourself(conn Connection) *CreateYourselfError {
	if IsR2(conn.Endpoint) {
		g := R2Guidance(conn.Bucket)
		return &CreateYourselfError{Provider: "r2", Guidance: g}
	}
	return &CreateYourselfError{
		Provider: "s3",
		Policy:   LeastPrivilegePolicy(conn.Bucket, conn.ServiceID),
		Guidance: fmt.Sprintf("create bucket %q, then attach this policy to the key so it can only reach %s", conn.Bucket, Prefix(conn.ServiceID)),
	}
}

// EnsureBucket creates the bucket only when the key is allowed to, and
// otherwise returns a *CreateYourselfError. An existing bucket is a no-op.
func EnsureBucket(ctx context.Context, conn Connection) error {
	cfg, err := conn.config(ctx)
	if err != nil {
		return err
	}
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	opts := s3.Options{
		Region:       region,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		HTTPClient:   netguard.NewClientAllowing(AllowPrivateEnv),
		UsePathStyle: cfg.PathStyle,
	}
	if cfg.Endpoint != "" {
		opts.BaseEndpoint = aws.String(cfg.Endpoint)
	}
	cl := s3.New(opts)

	ctx, cancel := context.WithTimeout(ctx, opTimeout())
	defer cancel()
	_, headErr := cl.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(cfg.Bucket)})
	if headErr == nil {
		return nil
	}
	// A prefix-scoped key gets 403 on HeadBucket, so only a transport or
	// credential failure stops here; 403 and 404 both fall through to create.
	if !isHTTPStatus(headErr, 403, 404) {
		return classify("head", headErr)
	}
	in := &s3.CreateBucketInput{Bucket: aws.String(cfg.Bucket)}
	if region != "us-east-1" && cfg.Endpoint == "" {
		in.CreateBucketConfiguration = &s3types.CreateBucketConfiguration{LocationConstraint: s3types.BucketLocationConstraint(region)}
	}
	_, err = cl.CreateBucket(ctx, in)
	if err == nil {
		return nil
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "BucketAlreadyOwnedByYou":
			return nil
		case "AccessDenied", "Forbidden", "AllAccessDisabled", "BucketAlreadyExists":
			return createYourself(conn)
		}
	}
	if isHTTPStatus(err, 403) {
		return createYourself(conn)
	}
	return classify("create", err)
}

func isHTTPStatus(err error, codes ...int) bool {
	var re interface{ HTTPStatusCode() int }
	if !errors.As(err, &re) {
		return false
	}
	for _, c := range codes {
		if re.HTTPStatusCode() == c {
			return true
		}
	}
	return false
}
