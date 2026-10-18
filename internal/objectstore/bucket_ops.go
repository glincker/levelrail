package objectstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const listPageSize int32 = 1000

// BucketPrefix is the key prefix a named bucket owns inside its destination.
func BucketPrefix(bucketID string) string { return "buckets/" + bucketID + "/" }

// ScopedKey joins a bucket prefix and a caller key, refusing keys that could escape the prefix.
func ScopedKey(prefix, key string) (string, error) {
	key = strings.TrimPrefix(key, "/")
	if key == "" || strings.Contains(key, "..") || strings.ContainsRune(key, 0) || len(key) > 1024 {
		return "", fmt.Errorf("objectstore: invalid object key")
	}
	return prefix + key, nil
}

// PresignGet returns a time-limited download URL for key.
func (c *Client) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	out, err := s3.NewPresignClient(c.s3).PresignGetObject(ctx,
		&s3.GetObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key)}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("objectstore: presign get %q: %w", key, err)
	}
	return out.URL, nil
}

// PresignPut returns a time-limited upload URL for key.
func (c *Client) PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error) {
	out, err := s3.NewPresignClient(c.s3).PresignPutObject(ctx,
		&s3.PutObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key)}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("objectstore: presign put %q: %w", key, err)
	}
	return out.URL, nil
}

// Usage totals the bytes and object count under prefix.
func (c *Client) Usage(ctx context.Context, prefix string) (bytes, objects int64, err error) {
	token := ""
	for {
		page, err := c.List(ctx, prefix, token, listPageSize)
		if err != nil {
			return 0, 0, err
		}
		for _, o := range page.Objects {
			bytes += o.Size
			objects++
		}
		if page.Next == "" {
			return bytes, objects, nil
		}
		token = page.Next
	}
}

// SweepResult reports what a lifecycle sweep removed.
type SweepResult struct {
	Deleted int64 `json:"deleted"`
	Bytes   int64 `json:"bytes"`
}

// SweepExpired deletes every object under prefix last modified before cutoff.
func (c *Client) SweepExpired(ctx context.Context, prefix string, cutoff time.Time) (SweepResult, error) {
	var res SweepResult
	token := ""
	for {
		page, err := c.List(ctx, prefix, token, listPageSize)
		if err != nil {
			return res, err
		}
		for _, o := range page.Objects {
			if !o.LastModified.Before(cutoff) {
				continue
			}
			if err := c.Delete(ctx, o.Key); err != nil {
				return res, err
			}
			res.Deleted++
			res.Bytes += o.Size
		}
		if page.Next == "" {
			return res, nil
		}
		token = page.Next
	}
}

// DeletePrefix removes every object under prefix.
func (c *Client) DeletePrefix(ctx context.Context, prefix string) (SweepResult, error) {
	return c.SweepExpired(ctx, prefix, time.Now().Add(24*time.Hour))
}
