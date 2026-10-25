package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/store"
)

type assetResource struct {
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	LastModified string `json:"last_modified,omitempty"`
	ContentType  string `json:"content_type,omitempty"`
	ETag         string `json:"etag,omitempty"`
	Uploader     string `json:"uploader,omitempty"`
	URL          string `json:"url,omitempty"`
}

type assetListResource struct {
	Assets []assetResource `json:"assets"`
	Cursor string          `json:"cursor,omitempty"`
}

// publicAssetURL is the URL to hand out for a key, or "" while the bucket is private or has no base URL.
// The provider key (prefix included) is used because a base URL maps to the provider bucket root.
func publicAssetURL(ab store.AssetBucket, key string) string {
	if ab.Settings.Visibility != store.AssetVisibilityPublic {
		return ""
	}
	base := ab.Settings.PublicBaseURL
	if ab.Settings.ServingMode == store.AssetServingCDN && ab.Settings.CDNBaseURL != "" {
		base = ab.Settings.CDNBaseURL
	}
	if base == "" {
		return ""
	}
	return objectstore.PublicObjectURL(base, ab.Bucket.Prefix+key)
}

// handleListAssets handles GET /api/v1/assets/buckets/{id}/assets?prefix=&cursor=&limit=. The provider listing is the truth;
// the index only adds who uploaded and the stored content type.
func (rt *Router) handleListAssets(w http.ResponseWriter, r *http.Request) {
	d, ab, ok := rt.loadAssetBucket(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), bucketOpTimeout)
	defer cancel()
	client, ok := rt.assetClient(ctx, w, d, ab)
	if !ok {
		return
	}
	limit := int32(bucketListLimit)
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v < bucketListLimit {
		limit = int32(v) //nolint:gosec // v is bounded above by bucketListLimit
	}
	page, err := client.List(ctx, ab.Bucket.Prefix+r.URL.Query().Get("prefix"), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		rt.internalError(w, "api: list assets failed", err, slog.String("bucket_id", ab.Bucket.ID))
		return
	}
	out := assetListResource{Assets: make([]assetResource, 0, len(page.Objects)), Cursor: page.Next}
	for _, o := range page.Objects {
		rel := o.Key[len(ab.Bucket.Prefix):]
		res := assetResource{Key: rel, Size: o.Size, LastModified: o.LastModified.UTC().Format(time.RFC3339), URL: publicAssetURL(ab, rel)}
		if row, err := d.Assets.GetAsset(ctx, ab.Bucket.ID, rel); err == nil {
			res.ContentType, res.ETag, res.Uploader = row.ContentType, row.ETag, row.Uploader
		}
		out.Assets = append(out.Assets, res)
	}
	writeJSON(w, http.StatusOK, out)
}

type assetUploadRequest struct {
	Key            string `json:"key"`
	ContentType    string `json:"content_type"`
	Size           int64  `json:"size"`
	ExpiresSeconds int    `json:"expires_seconds"`
}

type assetUploadResource struct {
	Key       string            `json:"key"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt string            `json:"expires_at"`
	MaxBytes  int64             `json:"max_bytes"`
}

// resolveContentType returns a valid media type, guessing from the extension and defaulting to octet-stream.
func resolveContentType(declared, key string) (string, bool) {
	if declared == "" {
		declared = mime.TypeByExtension(path.Ext(key))
	}
	if declared == "" {
		return "application/octet-stream", true
	}
	if _, _, err := mime.ParseMediaType(declared); err != nil {
		return "", false
	}
	return declared, true
}

// handleRequestAssetUpload handles POST /api/v1/assets/buckets/{id}/uploads: a presigned PUT that signs the
// declared Content-Type and Content-Length, so the browser cannot send a different type or a larger body.
func (rt *Router) handleRequestAssetUpload(w http.ResponseWriter, r *http.Request) {
	d, ab, ok := rt.loadAssetBucket(w, r)
	if !ok {
		return
	}
	var req assetUploadRequest
	if !decodeReq(w, r, &req) {
		return
	}
	key, err := objectstore.ScopedKey(ab.Bucket.Prefix, req.Key)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid key")
		return
	}
	contentType, valid := resolveContentType(req.ContentType, req.Key)
	if !valid {
		writeError(w, http.StatusBadRequest, "content_type is not a valid media type")
		return
	}
	maxBytes := assetsUploadMaxBytes()
	if req.Size <= 0 {
		writeError(w, http.StatusBadRequest, "size is required: it is signed into the URL")
		return
	}
	if req.Size > maxBytes {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("size exceeds the upload limit of %d bytes", maxBytes))
		return
	}
	ttl := assetsPresignMaxTTL()
	if req.ExpiresSeconds > 0 {
		if requested := time.Duration(req.ExpiresSeconds) * time.Second; requested < ttl {
			ttl = requested
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), bucketOpTimeout)
	defer cancel()
	client, ok := rt.assetClient(ctx, w, d, ab)
	if !ok {
		return
	}
	room, err := rt.bucketHasRoom(ctx, client, ab.Bucket, req.Size)
	if err != nil {
		rt.internalError(w, "api: asset upload: quota check failed", err, slog.String("bucket_id", ab.Bucket.ID))
		return
	}
	if !room {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("bucket quota of %d bytes would be exceeded: delete assets or raise quota_bytes", ab.Bucket.QuotaBytes))
		return
	}
	up, err := client.PresignUpload(ctx, objectstore.UploadRequest{Key: key, ContentType: contentType, Size: req.Size, TTL: ttl})
	if err != nil {
		rt.internalError(w, "api: asset presign upload failed", err, slog.String("bucket_id", ab.Bucket.ID))
		return
	}
	writeJSON(w, http.StatusOK, assetUploadResource{
		Key: req.Key, Method: up.Method, URL: up.URL, Headers: up.Headers,
		ExpiresAt: up.ExpiresAt.Format(time.RFC3339), MaxBytes: req.Size,
	})
}

type confirmUploadRequest struct {
	Key string `json:"key"`
}

// handleConfirmAssetUpload handles POST /api/v1/assets/buckets/{id}/uploads/confirm: a HEAD on the key, then the index row.
func (rt *Router) handleConfirmAssetUpload(w http.ResponseWriter, r *http.Request) {
	d, ab, ok := rt.loadAssetBucket(w, r)
	if !ok {
		return
	}
	var req confirmUploadRequest
	if !decodeReq(w, r, &req) {
		return
	}
	key, err := objectstore.ScopedKey(ab.Bucket.Prefix, req.Key)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid key")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), bucketOpTimeout)
	defer cancel()
	client, ok := rt.assetClient(ctx, w, d, ab)
	if !ok {
		return
	}
	info, err := client.Head(ctx, key)
	if errors.Is(err, objectstore.ErrObjectNotFound) {
		_ = d.Assets.DeleteAsset(ctx, ab.Bucket.ID, req.Key)
		writeError(w, http.StatusNotFound, "object not found: the upload may not have finished")
		return
	}
	if err != nil {
		rt.internalError(w, "api: confirm asset upload: head failed", err, slog.String("bucket_id", ab.Bucket.ID))
		return
	}
	rt.indexAsset(ctx, w, r, d, ab, req.Key, info, http.StatusOK)
}

func (rt *Router) indexAsset(ctx context.Context, w http.ResponseWriter, r *http.Request, d assetsDeps, ab store.AssetBucket, rel string, info objectstore.ObjectInfo, status int) {
	ptype, pid, _, err := rt.callerPrincipal(r)
	if err != nil {
		rt.internalError(w, "api: index asset: resolve caller failed", err)
		return
	}
	row, err := d.Assets.UpsertAsset(ctx, store.Asset{
		BucketID: ab.Bucket.ID, Key: rel, SizeBytes: info.Size, ContentType: info.ContentType, ETag: info.ETag, Uploader: ptype + ":" + pid,
	})
	if err != nil {
		rt.internalError(w, "api: index asset failed", err, slog.String("bucket_id", ab.Bucket.ID))
		return
	}
	writeJSON(w, status, assetResource{
		Key: rel, Size: row.SizeBytes, ContentType: row.ContentType, ETag: row.ETag, Uploader: row.Uploader,
		LastModified: info.LastModified.UTC().Format(time.RFC3339), URL: publicAssetURL(ab, rel),
	})
}

// handleUploadAsset handles PUT /api/v1/assets/buckets/{id}/objects?key=: the raw body becomes the object. Bodies over
// the API body limit get a 413 that names the presign route.
func (rt *Router) handleUploadAsset(w http.ResponseWriter, r *http.Request) {
	d, ab, ok := rt.loadAssetBucket(w, r)
	if !ok {
		return
	}
	rel := r.URL.Query().Get("key")
	key, err := objectstore.ScopedKey(ab.Bucket.Prefix, rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid key")
		return
	}
	contentType, valid := resolveContentType(r.Header.Get("Content-Type"), rel)
	if !valid {
		writeError(w, http.StatusBadRequest, "Content-Type is not a valid media type")
		return
	}
	tmp, err := os.CreateTemp("", "asset-upload-*")
	if err != nil {
		rt.internalError(w, "api: asset upload: temp file failed", err)
		return
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	maxBytes := assetsUploadMaxBytes()
	size, err := io.Copy(tmp, io.LimitReader(r.Body, maxBytes+1))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "file is too large for a direct upload: request a presigned upload at POST .../uploads")
			return
		}
		writeError(w, http.StatusBadRequest, "could not read upload")
		return
	}
	if size > maxBytes {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("size exceeds the upload limit of %d bytes", maxBytes))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), bucketOpTimeout)
	defer cancel()
	client, ok := rt.assetClient(ctx, w, d, ab)
	if !ok {
		return
	}
	room, err := rt.bucketHasRoom(ctx, client, ab.Bucket, size)
	if err != nil {
		rt.internalError(w, "api: asset upload: quota check failed", err, slog.String("bucket_id", ab.Bucket.ID))
		return
	}
	if !room {
		writeError(w, http.StatusRequestEntityTooLarge, "bucket quota exceeded")
		return
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		rt.internalError(w, "api: asset upload: rewind failed", err)
		return
	}
	if err := client.Put(ctx, key, tmp, contentType, ""); err != nil {
		rt.internalError(w, "api: asset upload failed", err, slog.String("bucket_id", ab.Bucket.ID))
		return
	}
	info, err := client.Head(ctx, key)
	if err != nil {
		info = objectstore.ObjectInfo{Size: size, ContentType: contentType, LastModified: time.Now()}
	}
	rt.indexAsset(ctx, w, r, d, ab, rel, info, http.StatusCreated)
}

// handleDeleteAsset handles DELETE /api/v1/assets/buckets/{id}/objects?key=.
func (rt *Router) handleDeleteAsset(w http.ResponseWriter, r *http.Request) {
	d, ab, ok := rt.loadAssetBucket(w, r)
	if !ok {
		return
	}
	rel := r.URL.Query().Get("key")
	key, err := objectstore.ScopedKey(ab.Bucket.Prefix, rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid key")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), bucketOpTimeout)
	defer cancel()
	client, ok := rt.assetClient(ctx, w, d, ab)
	if !ok {
		return
	}
	if err := client.Delete(ctx, key); err != nil {
		rt.internalError(w, "api: delete asset failed", err, slog.String("bucket_id", ab.Bucket.ID))
		return
	}
	if err := d.Assets.DeleteAsset(ctx, ab.Bucket.ID, rel); err != nil {
		rt.internalError(w, "api: delete asset index failed", err, slog.String("bucket_id", ab.Bucket.ID))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type assetURLsResource struct {
	Key              string   `json:"key"`
	Visibility       string   `json:"visibility"`
	OriginURL        string   `json:"origin_url,omitempty"`
	CDNURL           string   `json:"cdn_url,omitempty"`
	PresignedURL     string   `json:"presigned_url,omitempty"`
	PresignedExpires string   `json:"presigned_expires_at,omitempty"`
	Notes            []string `json:"notes"`
}

// handleAssetURLs handles GET /api/v1/assets/buckets/{id}/urls?key=&expires_seconds=. A private bucket only gets a
// presigned GET; base URLs are never guessed, only built from what the operator configured.
func (rt *Router) handleAssetURLs(w http.ResponseWriter, r *http.Request) {
	d, ab, ok := rt.loadAssetBucket(w, r)
	if !ok {
		return
	}
	rel := r.URL.Query().Get("key")
	key, err := objectstore.ScopedKey(ab.Bucket.Prefix, rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid key")
		return
	}
	res := assetURLsResource{Key: rel, Visibility: ab.Settings.Visibility, Notes: []string{}}
	if ab.Settings.Visibility == store.AssetVisibilityPublic {
		if ab.Settings.PublicBaseURL != "" {
			res.OriginURL = objectstore.PublicObjectURL(ab.Settings.PublicBaseURL, key)
		}
		if ab.Settings.CDNBaseURL != "" {
			res.CDNURL = objectstore.PublicObjectURL(ab.Settings.CDNBaseURL, key)
		}
		if res.OriginURL == "" && res.CDNURL == "" {
			res.Notes = append(res.Notes, "no public_base_url or cdn_base_url is set, so only a presigned URL is available")
		}
	} else {
		res.Notes = append(res.Notes, "the bucket is private: only a time-limited presigned URL is handed out")
	}
	if ab.Settings.Visibility == store.AssetVisibilityPrivate || (res.OriginURL == "" && res.CDNURL == "") {
		ttl := assetsPresignMaxTTL()
		if v, err := strconv.Atoi(r.URL.Query().Get("expires_seconds")); err == nil && v > 0 && time.Duration(v)*time.Second < ttl {
			ttl = time.Duration(v) * time.Second
		}
		ctx, cancel := context.WithTimeout(r.Context(), bucketOpTimeout)
		defer cancel()
		client, ok := rt.assetClient(ctx, w, d, ab)
		if !ok {
			return
		}
		url, err := client.PresignGet(ctx, key, ttl)
		if err != nil {
			rt.internalError(w, "api: asset presign get failed", err, slog.String("bucket_id", ab.Bucket.ID))
			return
		}
		res.PresignedURL, res.PresignedExpires = url, time.Now().Add(ttl).UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, res)
}
