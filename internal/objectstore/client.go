package objectstore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/GLINCKER/levelrail/internal/netguard"
)

// Config is everything needed to talk to one bucket.
type Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	PathStyle       bool
	// MaxAttempts bounds SDK retries per request; zero means 3.
	MaxAttempts int
	// HTTPClient defaults to netguard.NewClient(), which refuses internal addresses.
	HTTPClient *http.Client
}

// Object is one listed key.
type Object struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
}

// ListPage is one page of a prefix listing.
type ListPage struct {
	Objects []Object `json:"objects"`
	Next    string   `json:"next,omitempty"`
}

// Client is a bucket-scoped S3 client.
type Client struct {
	s3     *s3.Client
	bucket string
}

// New builds a Client from static credentials.
func New(cfg Config) (*Client, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("objectstore: bucket is required")
	}
	if cfg.Endpoint != "" {
		if err := ValidateEndpoint(cfg.Endpoint); err != nil {
			return nil, fmt.Errorf("objectstore: %w", err)
		}
	}
	region := cfg.Region
	if region == "" {
		region = defaultRegion
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = netguard.NewClient()
	}
	attempts := cfg.MaxAttempts
	if attempts <= 0 {
		attempts = 3
	}
	opts := s3.Options{
		Region:                     region,
		Credentials:                credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		HTTPClient:                 httpClient,
		UsePathStyle:               cfg.PathStyle,
		RetryMaxAttempts:           attempts,
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	}
	if cfg.Endpoint != "" {
		opts.BaseEndpoint = aws.String(cfg.Endpoint)
	}
	return &Client{s3: s3.New(opts), bucket: cfg.Bucket}, nil
}

// Put uploads body under key. The body must be seekable so the SDK can sign
// and retry it.
func (c *Client) Put(ctx context.Context, key string, body io.ReadSeeker, contentType, contentEncoding string) error {
	in := &s3.PutObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key), Body: body}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	if contentEncoding != "" {
		in.ContentEncoding = aws.String(contentEncoding)
	}
	if _, err := c.s3.PutObject(ctx, in); err != nil {
		return fmt.Errorf("objectstore: put %q: %w", key, err)
	}
	return nil
}

// Get opens an object. The caller closes the returned reader.
func (c *Client) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key)})
	if err != nil {
		return nil, 0, fmt.Errorf("objectstore: get %q: %w", key, err)
	}
	return out.Body, aws.ToInt64(out.ContentLength), nil
}

// Delete removes an object; deleting a missing key is not an error on S3.
func (c *Client) Delete(ctx context.Context, key string) error {
	if _, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key)}); err != nil {
		return fmt.Errorf("objectstore: delete %q: %w", key, err)
	}
	return nil
}

// List returns up to limit keys under prefix, resuming from a previous page's Next token.
func (c *Client) List(ctx context.Context, prefix, token string, limit int32) (ListPage, error) {
	in := &s3.ListObjectsV2Input{Bucket: aws.String(c.bucket), Prefix: aws.String(prefix), MaxKeys: aws.Int32(limit)}
	if token != "" {
		in.ContinuationToken = aws.String(token)
	}
	out, err := c.s3.ListObjectsV2(ctx, in)
	if err != nil {
		return ListPage{}, fmt.Errorf("objectstore: list %q: %w", prefix, err)
	}
	page := ListPage{Objects: make([]Object, 0, len(out.Contents))}
	for _, o := range out.Contents {
		page.Objects = append(page.Objects, Object{
			Key: aws.ToString(o.Key), Size: aws.ToInt64(o.Size), LastModified: aws.ToTime(o.LastModified),
		})
	}
	if aws.ToBool(out.IsTruncated) {
		page.Next = aws.ToString(out.NextContinuationToken)
	}
	return page, nil
}
