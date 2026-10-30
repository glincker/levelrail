package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/kit/httpx"
)

// AssetBucket is an asset bucket as the API returns it; credentials never appear.
type AssetBucket struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	TargetID        string   `json:"target_id"`
	Preset          string   `json:"preset,omitempty"`
	Prefix          string   `json:"prefix"`
	ProjectID       string   `json:"project_id,omitempty"`
	Visibility      string   `json:"visibility"`
	ServingMode     string   `json:"serving_mode"`
	PublicBaseURL   string   `json:"public_base_url,omitempty"`
	CDNBaseURL      string   `json:"cdn_base_url,omitempty"`
	ReferrerAllow   []string `json:"referrer_allow"`
	RefuseNoReferer bool     `json:"refuse_no_referer"`
	QuotaBytes      int64    `json:"quota_bytes"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

// AssetBucketCreate is the body of POST /api/v1/assets/buckets.
type AssetBucketCreate struct {
	Name           string   `json:"name"`
	TargetID       string   `json:"target_id"`
	Prefix         string   `json:"prefix,omitempty"`
	ProjectID      string   `json:"project_id,omitempty"`
	Visibility     string   `json:"visibility,omitempty"`
	ConfirmName    string   `json:"confirm_name,omitempty"`
	ServingMode    string   `json:"serving_mode,omitempty"`
	PublicBaseURL  string   `json:"public_base_url,omitempty"`
	CDNBaseURL     string   `json:"cdn_base_url,omitempty"`
	ReferrerAllow  []string `json:"referrer_allow,omitempty"`
	AllowNoReferer *bool    `json:"allow_no_referer,omitempty"`
	QuotaBytes     int64    `json:"quota_bytes,omitempty"`
}

// AssetBucketUpdate is the PATCH body; a nil field is left unchanged.
type AssetBucketUpdate struct {
	ProjectID      *string   `json:"project_id,omitempty"`
	Visibility     *string   `json:"visibility,omitempty"`
	ConfirmName    string    `json:"confirm_name,omitempty"`
	ServingMode    *string   `json:"serving_mode,omitempty"`
	PublicBaseURL  *string   `json:"public_base_url,omitempty"`
	CDNBaseURL     *string   `json:"cdn_base_url,omitempty"`
	ReferrerAllow  *[]string `json:"referrer_allow,omitempty"`
	AllowNoReferer *bool     `json:"allow_no_referer,omitempty"`
	QuotaBytes     *int64    `json:"quota_bytes,omitempty"`
}

// AssetProbeStep is one step of an asset bucket probe.
type AssetProbeStep struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// AssetProbeCORS is the browser upload CORS step.
type AssetProbeCORS struct {
	Status  string `json:"status"`
	Origin  string `json:"origin"`
	Message string `json:"message,omitempty"`
}

// AssetProbeExposure is the anonymous read check against the public base URL.
type AssetProbeExposure struct {
	URL    string `json:"url"`
	Public bool   `json:"public"`
	Status int    `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

// AssetProbeWarning is a typed finding with an optional fix name.
type AssetProbeWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
}

// AssetProbe is the result of POST .../probe.
type AssetProbe struct {
	OK       bool                `json:"ok"`
	Reason   string              `json:"reason,omitempty"`
	Message  string              `json:"message,omitempty"`
	Fix      string              `json:"fix,omitempty"`
	Steps    []AssetProbeStep    `json:"steps"`
	CORS     *AssetProbeCORS     `json:"cors,omitempty"`
	Exposure *AssetProbeExposure `json:"exposure,omitempty"`
	Warnings []AssetProbeWarning `json:"warnings"`
}

// AssetCostLine prices one provider preset.
type AssetCostLine struct {
	Preset       string  `json:"preset"`
	StorageUSD   float64 `json:"storage_usd"`
	EgressUSD    float64 `json:"egress_usd"`
	TotalUSD     float64 `json:"total_usd"`
	FreeEgressGB float64 `json:"free_egress_gb"`
}

// AssetCost is always an estimate from list prices dated PriceTableDate.
type AssetCost struct {
	Estimated      bool            `json:"estimated"`
	PriceTableDate string          `json:"price_table_date"`
	StoredBytes    int64           `json:"stored_bytes"`
	EgressGB       float64         `json:"egress_gb"`
	Current        AssetCostLine   `json:"current"`
	Compare        []AssetCostLine `json:"compare"`
	OverrideEnvs   []string        `json:"override_envs"`
	Notes          []string        `json:"notes"`
}

// AssetUsage is the result of GET .../usage.
type AssetUsage struct {
	Bytes           int64     `json:"bytes"`
	Objects         int64     `json:"objects"`
	AtLeast         bool      `json:"at_least"`
	QuotaBytes      int64     `json:"quota_bytes"`
	ServingMode     string    `json:"serving_mode"`
	TrafficMeasured bool      `json:"traffic_measured"`
	Cost            AssetCost `json:"cost"`
}

// Asset is one object in a bucket listing or an upload result.
type Asset struct {
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	LastModified string `json:"last_modified,omitempty"`
	ContentType  string `json:"content_type,omitempty"`
	ETag         string `json:"etag,omitempty"`
	Uploader     string `json:"uploader,omitempty"`
	URL          string `json:"url,omitempty"`
}

// AssetList is one page of objects; Cursor is empty on the last page.
type AssetList struct {
	Assets []Asset `json:"assets"`
	Cursor string  `json:"cursor,omitempty"`
}

// AssetUploadRequest asks for a presigned PUT; Size is signed into the URL.
type AssetUploadRequest struct {
	Key            string `json:"key"`
	ContentType    string `json:"content_type,omitempty"`
	Size           int64  `json:"size"`
	ExpiresSeconds int    `json:"expires_seconds,omitempty"`
}

// AssetUpload is the presigned PUT to run against the provider.
type AssetUpload struct {
	Key       string            `json:"key"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt string            `json:"expires_at"`
	MaxBytes  int64             `json:"max_bytes"`
}

// AssetURLs is the result of GET .../urls.
type AssetURLs struct {
	Key              string   `json:"key"`
	Visibility       string   `json:"visibility"`
	OriginURL        string   `json:"origin_url,omitempty"`
	CDNURL           string   `json:"cdn_url,omitempty"`
	PresignedURL     string   `json:"presigned_url,omitempty"`
	PresignedExpires string   `json:"presigned_expires_at,omitempty"`
	Notes            []string `json:"notes"`
}

func assetsPath(parts ...string) string {
	p := "/api/v1/assets/buckets"
	for _, s := range parts {
		p += "/" + url.PathEscape(s)
	}
	return p
}

// assetCall returns the server JSON untouched for --json plus the typed decode.
func assetCall[T any](ctx context.Context, c *Client, method, path string, body any) (T, json.RawMessage, error) {
	var zero, out T
	var raw json.RawMessage
	if err := c.do(ctx, method, path, body, &raw); err != nil {
		return zero, nil, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			return zero, raw, fmt.Errorf("decode assets response: %w", err)
		}
	}
	return out, raw, nil
}

// ListAssetBuckets calls GET /api/v1/assets/buckets, optionally for one project.
func (c *Client) ListAssetBuckets(ctx context.Context, project string) ([]AssetBucket, json.RawMessage, error) {
	p := assetsPath()
	if project != "" {
		p += "?project=" + url.QueryEscape(project)
	}
	return assetCall[[]AssetBucket](ctx, c, http.MethodGet, p, nil)
}

// GetAssetBucket calls GET /api/v1/assets/buckets/{id}.
func (c *Client) GetAssetBucket(ctx context.Context, id string) (AssetBucket, json.RawMessage, error) {
	return assetCall[AssetBucket](ctx, c, http.MethodGet, assetsPath(id), nil)
}

// CreateAssetBucket calls POST /api/v1/assets/buckets.
func (c *Client) CreateAssetBucket(ctx context.Context, req AssetBucketCreate) (AssetBucket, json.RawMessage, error) {
	return assetCall[AssetBucket](ctx, c, http.MethodPost, assetsPath(), req)
}

// UpdateAssetBucket calls PATCH /api/v1/assets/buckets/{id}.
func (c *Client) UpdateAssetBucket(ctx context.Context, id string, req AssetBucketUpdate) (AssetBucket, json.RawMessage, error) {
	return assetCall[AssetBucket](ctx, c, http.MethodPatch, assetsPath(id), req)
}

// DeleteAssetBucket calls DELETE /api/v1/assets/buckets/{id}; objects stay at the provider.
func (c *Client) DeleteAssetBucket(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, assetsPath(id), nil, nil)
}

// ProbeAssetBucket calls POST .../probe. An empty origin lets the server use the dashboard URL.
func (c *Client) ProbeAssetBucket(ctx context.Context, id, origin string, fixCORS bool) (AssetProbe, json.RawMessage, error) {
	body := struct {
		Origin  string `json:"origin,omitempty"`
		FixCORS bool   `json:"fix_cors"`
	}{origin, fixCORS}
	return assetCall[AssetProbe](ctx, c, http.MethodPost, assetsPath(id, "probe"), body)
}

// AssetBucketUsage calls GET .../usage. An egressGB below zero is not sent.
func (c *Client) AssetBucketUsage(ctx context.Context, id string, egressGB float64) (AssetUsage, json.RawMessage, error) {
	p := assetsPath(id, "usage")
	if egressGB >= 0 {
		p += "?egress_gb=" + strconv.FormatFloat(egressGB, 'f', -1, 64)
	}
	return assetCall[AssetUsage](ctx, c, http.MethodGet, p, nil)
}

// ListAssets calls GET .../assets.
func (c *Client) ListAssets(ctx context.Context, id, prefix, cursor string, limit int) (AssetList, json.RawMessage, error) {
	q := url.Values{}
	if prefix != "" {
		q.Set("prefix", prefix)
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	p := assetsPath(id, "assets")
	if len(q) > 0 {
		p += "?" + q.Encode()
	}
	return assetCall[AssetList](ctx, c, http.MethodGet, p, nil)
}

// UploadAssetDirect calls PUT .../objects?key= with the raw body. It never retries
// because the body is a stream that cannot be replayed.
func (c *Client) UploadAssetDirect(ctx context.Context, id, key, contentType string, size int64, body io.Reader) (Asset, json.RawMessage, error) {
	var zero Asset
	if size == 0 {
		body = http.NoBody
	}
	u := c.baseURL + assetsPath(id, "objects") + "?key=" + url.QueryEscape(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, body) //nolint:gosec // baseURL is the operator-supplied API target
	if err != nil {
		return zero, nil, fmt.Errorf("build request: %w", err)
	}
	req.ContentLength = size
	req.Header.Set(headerContentType, contentType)
	if c.token != "" {
		req.Header.Set(headerAuthorization, bearerPrefix+c.token)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	resp, err := c.hc.Do(req) //nolint:gosec // same target as above
	if err != nil {
		return zero, nil, fmt.Errorf("request PUT %s: %w", u, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var raw json.RawMessage
	if err := decodeResponse(resp, &raw); err != nil {
		return zero, nil, err
	}
	var out Asset
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			return zero, raw, fmt.Errorf("decode assets response: %w", err)
		}
	}
	return out, raw, nil
}

// RequestAssetUpload calls POST .../uploads.
func (c *Client) RequestAssetUpload(ctx context.Context, id string, req AssetUploadRequest) (AssetUpload, json.RawMessage, error) {
	return assetCall[AssetUpload](ctx, c, http.MethodPost, assetsPath(id, "uploads"), req)
}

// ConfirmAssetUpload calls POST .../uploads/confirm once the presigned PUT finished.
func (c *Client) ConfirmAssetUpload(ctx context.Context, id, key string) (Asset, json.RawMessage, error) {
	return assetCall[Asset](ctx, c, http.MethodPost, assetsPath(id, "uploads", "confirm"), struct {
		Key string `json:"key"`
	}{key})
}

// presignedClient skips the control plane transports so scope headers never reach the provider.
var presignedClient = &http.Client{Timeout: 30 * time.Minute}

// PutPresigned runs the PUT that RequestAssetUpload returned: the signed headers, the exact body size,
// and no control plane credentials, since the URL carries its own signature.
func (c *Client) PutPresigned(ctx context.Context, up AssetUpload, size int64, body io.Reader) error {
	method := up.Method
	if method == "" {
		method = http.MethodPut
	}
	if size == 0 {
		body = http.NoBody
	}
	req, err := http.NewRequestWithContext(ctx, method, up.URL, body)
	if err != nil {
		return fmt.Errorf("build presigned request: %w", err)
	}
	req.ContentLength = size
	for k, v := range up.Headers {
		req.Header.Set(k, v)
	}
	resp, err := presignedClient.Do(req) //nolint:gosec // URL is the provider URL the control plane just signed
	if err != nil {
		return fmt.Errorf("presigned upload: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := httpx.ReadBody(resp.Body)
		return fmt.Errorf("provider rejected the upload (status %d): %s", resp.StatusCode, truncateForError(string(data)))
	}
	return nil
}

func truncateForError(s string) string {
	const limit = 300
	if len(s) > limit {
		return s[:limit]
	}
	return s
}

// AssetURLs calls GET .../urls?key=.
func (c *Client) AssetURLs(ctx context.Context, id, key string, expiresSeconds int) (AssetURLs, json.RawMessage, error) {
	q := url.Values{"key": {key}}
	if expiresSeconds > 0 {
		q.Set("expires_seconds", strconv.Itoa(expiresSeconds))
	}
	return assetCall[AssetURLs](ctx, c, http.MethodGet, assetsPath(id, "urls")+"?"+q.Encode(), nil)
}

// DeleteAsset calls DELETE .../objects?key=.
func (c *Client) DeleteAsset(ctx context.Context, id, key string) error {
	return c.do(ctx, http.MethodDelete, assetsPath(id, "objects")+"?key="+url.QueryEscape(key), nil, nil)
}
