package objectstore

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// UploadRequest describes one browser-direct upload. Content type and size are signed into the URL.
type UploadRequest struct {
	Key         string
	ContentType string
	Size        int64
	TTL         time.Duration
}

// PresignedUpload is a PUT URL plus the headers the client must send for the signature to hold.
type PresignedUpload struct {
	URL       string            `json:"url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// PresignUpload signs a PUT that only accepts the declared Content-Type and Content-Length.
func (c *Client) PresignUpload(ctx context.Context, req UploadRequest) (PresignedUpload, error) {
	if req.Size <= 0 {
		return PresignedUpload{}, errors.New("objectstore: presign upload needs a positive size")
	}
	if req.TTL <= 0 {
		return PresignedUpload{}, errors.New("objectstore: presign upload needs a positive ttl")
	}
	if req.ContentType == "" || strings.ContainsAny(req.ContentType, "\r\n") {
		return PresignedUpload{}, errors.New("objectstore: presign upload needs a valid content type")
	}
	out, err := s3.NewPresignClient(c.s3).PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(req.Key),
		ContentType:   aws.String(req.ContentType),
		ContentLength: aws.Int64(req.Size),
	}, s3.WithPresignExpires(req.TTL))
	if err != nil {
		return PresignedUpload{}, fmt.Errorf("objectstore: presign upload %q: %w", req.Key, err)
	}
	return PresignedUpload{
		URL:       out.URL,
		Method:    http.MethodPut,
		Headers:   map[string]string{"Content-Type": req.ContentType},
		ExpiresAt: time.Now().Add(req.TTL).UTC(),
	}, nil
}

// ObjectInfo is the result of a HEAD on one key.
type ObjectInfo struct {
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
}

// ErrObjectNotFound is returned by Head when the key does not exist.
var ErrObjectNotFound = errors.New("objectstore: object not found")

// Head returns an object's size, type and ETag, or ErrObjectNotFound.
func (c *Client) Head(ctx context.Context, key string) (ObjectInfo, error) {
	out, err := c.s3.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key)})
	if err != nil {
		var respErr interface{ HTTPStatusCode() int }
		if errors.As(err, &respErr) && respErr.HTTPStatusCode() == http.StatusNotFound {
			return ObjectInfo{}, ErrObjectNotFound
		}
		return ObjectInfo{}, fmt.Errorf("objectstore: head %q: %w", key, err)
	}
	return ObjectInfo{
		Size: aws.ToInt64(out.ContentLength), ContentType: aws.ToString(out.ContentType),
		ETag: strings.Trim(aws.ToString(out.ETag), `"`), LastModified: aws.ToTime(out.LastModified),
	}, nil
}

// UsageBounded totals bytes and objects under prefix but stops after maxObjects, reporting truncated so callers say "at least".
func (c *Client) UsageBounded(ctx context.Context, prefix string, maxObjects int64) (bytes, objects int64, truncated bool, err error) {
	token := ""
	for {
		page, err := c.List(ctx, prefix, token, listPageSize)
		if err != nil {
			return 0, 0, false, err
		}
		for _, o := range page.Objects {
			if maxObjects > 0 && objects >= maxObjects {
				return bytes, objects, true, nil
			}
			bytes += o.Size
			objects++
		}
		if page.Next == "" {
			return bytes, objects, false, nil
		}
		token = page.Next
	}
}
