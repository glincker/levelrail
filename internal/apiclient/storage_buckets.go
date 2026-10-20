package apiclient

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// BucketAppRef is one app a bucket is bound to.
type BucketAppRef struct {
	AppName   string `json:"app_name"`
	EnvPrefix string `json:"env_prefix"`
}

// StorageBucket is a named bucket on a storage destination.
type StorageBucket struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	TargetID      string         `json:"target_id"`
	Prefix        string         `json:"prefix"`
	Public        bool           `json:"public"`
	PublicURL     string         `json:"public_url,omitempty"`
	QuotaBytes    int64          `json:"quota_bytes"`
	LifecycleDays int            `json:"lifecycle_days"`
	LastSweptAt   string         `json:"last_swept_at,omitempty"`
	CreatedAt     string         `json:"created_at"`
	Apps          []BucketAppRef `json:"apps"`
	EnvKeys       []string       `json:"env_keys,omitempty"`
}

// CreateStorageBucketRequest is the body of POST /api/v1/storage/buckets.
type CreateStorageBucketRequest struct {
	Name          string `json:"name"`
	TargetID      string `json:"target_id"`
	AppName       string `json:"app_name,omitempty"`
	EnvPrefix     string `json:"env_prefix,omitempty"`
	Public        bool   `json:"public,omitempty"`
	QuotaBytes    int64  `json:"quota_bytes,omitempty"`
	LifecycleDays int    `json:"lifecycle_days,omitempty"`
}

// UpdateStorageBucketRequest changes only the fields that are set.
type UpdateStorageBucketRequest struct {
	Public        *bool  `json:"public,omitempty"`
	QuotaBytes    *int64 `json:"quota_bytes,omitempty"`
	LifecycleDays *int   `json:"lifecycle_days,omitempty"`
}

// BucketObject is one object in a bucket.
type BucketObject struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
}

// BucketObjects is one page of a bucket listing.
type BucketObjects struct {
	Objects []BucketObject `json:"objects"`
	Next    string         `json:"next,omitempty"`
}

// BucketUsage is the stored size of a bucket against its quota.
type BucketUsage struct {
	Bytes      int64 `json:"bytes"`
	Objects    int64 `json:"objects"`
	QuotaBytes int64 `json:"quota_bytes"`
}

// BucketPresign is a time-limited URL for one object.
type BucketPresign struct {
	URL       string `json:"url"`
	Method    string `json:"method"`
	ExpiresAt string `json:"expires_at"`
}

// BucketSweepResult reports what a lifecycle sweep removed.
type BucketSweepResult struct {
	Deleted int64 `json:"deleted"`
	Bytes   int64 `json:"bytes"`
}

func bucketPath(id string) string { return "/api/v1/storage/buckets/" + PathEscape(id) }

// ListStorageBuckets calls GET /api/v1/storage/buckets, filtered to one app when app is set.
func (c *Client) ListStorageBuckets(ctx context.Context, app string) ([]StorageBucket, error) {
	var out []StorageBucket
	path := "/api/v1/storage/buckets"
	if app != "" {
		path += "?app=" + url.QueryEscape(app)
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// CreateStorageBucket calls POST /api/v1/storage/buckets.
func (c *Client) CreateStorageBucket(ctx context.Context, req CreateStorageBucketRequest) (StorageBucket, error) {
	var out StorageBucket
	err := c.do(ctx, http.MethodPost, "/api/v1/storage/buckets", req, &out)
	return out, err
}

// UpdateStorageBucket calls PATCH /api/v1/storage/buckets/{id}.
func (c *Client) UpdateStorageBucket(ctx context.Context, id string, req UpdateStorageBucketRequest) (StorageBucket, error) {
	var out StorageBucket
	err := c.do(ctx, http.MethodPatch, bucketPath(id), req, &out)
	return out, err
}

// DeleteStorageBucket calls DELETE /api/v1/storage/buckets/{id}.
func (c *Client) DeleteStorageBucket(ctx context.Context, id string, purge bool) error {
	path := bucketPath(id)
	if purge {
		path += "?purge=true"
	}
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// BindAppBucket calls PUT /api/v1/apps/{name}/buckets/{id}.
func (c *Client) BindAppBucket(ctx context.Context, app, id, envPrefix string) (StorageBucket, error) {
	var out StorageBucket
	body := map[string]string{"env_prefix": envPrefix}
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(app)+"/buckets/"+PathEscape(id), body, &out)
	return out, err
}

// UnbindAppBucket calls DELETE /api/v1/apps/{name}/buckets/{id}.
func (c *Client) UnbindAppBucket(ctx context.Context, app, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/apps/"+PathEscape(app)+"/buckets/"+PathEscape(id), nil, nil)
}

// ListBucketObjects calls GET /api/v1/storage/buckets/{id}/objects.
func (c *Client) ListBucketObjects(ctx context.Context, id, prefix, token string) (BucketObjects, error) {
	var out BucketObjects
	q := url.Values{"prefix": {prefix}, "token": {token}}
	err := c.do(ctx, http.MethodGet, bucketPath(id)+"/objects?"+q.Encode(), nil, &out)
	return out, err
}

// BucketUsage calls GET /api/v1/storage/buckets/{id}/usage.
func (c *Client) BucketUsage(ctx context.Context, id string) (BucketUsage, error) {
	var out BucketUsage
	err := c.do(ctx, http.MethodGet, bucketPath(id)+"/usage", nil, &out)
	return out, err
}

// DownloadBucketObject calls GET /api/v1/storage/buckets/{id}/objects/download.
func (c *Client) DownloadBucketObject(ctx context.Context, id, key string) ([]byte, error) {
	return c.downloadRaw(ctx, bucketPath(id)+"/objects/download?"+url.Values{"key": {key}}.Encode())
}

// DeleteBucketObject calls DELETE /api/v1/storage/buckets/{id}/objects.
func (c *Client) DeleteBucketObject(ctx context.Context, id, key string) error {
	return c.do(ctx, http.MethodDelete, bucketPath(id)+"/objects?"+url.Values{"key": {key}}.Encode(), nil, nil)
}

// PresignBucketObject calls POST /api/v1/storage/buckets/{id}/presign.
func (c *Client) PresignBucketObject(ctx context.Context, id, key, method string, expiresSeconds int) (BucketPresign, error) {
	var out BucketPresign
	body := map[string]any{"key": key, "method": method, "expires_seconds": expiresSeconds}
	err := c.do(ctx, http.MethodPost, bucketPath(id)+"/presign", body, &out)
	return out, err
}

// RunBucketLifecycle calls POST /api/v1/storage/buckets/{id}/lifecycle/run.
func (c *Client) RunBucketLifecycle(ctx context.Context, id string) (BucketSweepResult, error) {
	var out BucketSweepResult
	err := c.do(ctx, http.MethodPost, bucketPath(id)+"/lifecycle/run", nil, &out)
	return out, err
}

// UploadBucketObject calls PUT /api/v1/storage/buckets/{id}/objects with data as the raw body.
func (c *Client) UploadBucketObject(ctx context.Context, id, key string, data []byte) (BucketObject, error) {
	var out BucketObject
	path := bucketPath(id) + "/objects?" + url.Values{"key": {key}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+path, bytes.NewReader(data)) //nolint:gosec // c.baseURL is the operator-supplied API target, same as do()
	if err != nil {
		return out, fmt.Errorf("build request: %w", err)
	}
	if c.token != "" {
		req.Header.Set(headerAuthorization, bearerPrefix+c.token)
	}
	start := time.Now()
	resp, err := c.hc.Do(req) //nolint:gosec // same target as do()
	if err != nil {
		TraceRequest(http.MethodPut, req.URL.String(), "", time.Since(start), err)
		return out, fmt.Errorf("request %s %s: %w", http.MethodPut, req.URL.String(), err)
	}
	defer func() { _ = resp.Body.Close() }()
	TraceRequest(http.MethodPut, req.URL.String(), resp.Status, time.Since(start), nil)
	return out, decodeResponse(resp, &out)
}
