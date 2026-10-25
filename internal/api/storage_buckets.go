package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultBucketEnvPrefix    = "S3"
	defaultPresignSeconds     = 900
	maxPresignSeconds         = 7 * 24 * 3600
	maxBucketLifecycleDays    = 3650
	bucketListLimit           = 200
	bucketOpTimeout           = 2 * time.Minute
	publicBucketPathPrefix    = "/api/v1/public/buckets/"
	envPublicBucketRatePerMin = "APP_PUBLIC_BUCKET_RATE_PER_MIN"
	envPublicBucketMaxBytes   = "APP_PUBLIC_BUCKET_MAX_BYTES"

	defaultPublicBucketRatePerMin       = 120
	defaultPublicBucketMaxBytes   int64 = 100 << 20
	bucketObjectSandboxPolicy           = "sandbox; default-src 'none'"
	storageBucketIDPrefix               = "sb_"
)

var (
	bucketNameRE      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)
	bucketEnvPrefixRE = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,31}$`)
)

// BucketRowStore persists named buckets and their app bindings.
type BucketRowStore interface {
	CreateStorageBucket(ctx context.Context, b store.StorageBucket) (store.StorageBucket, error)
	GetStorageBucket(ctx context.Context, id string) (store.StorageBucket, error)
	ListStorageBuckets(ctx context.Context) ([]store.StorageBucket, error)
	UpdateStorageBucketSettings(ctx context.Context, id string, public bool, quotaBytes int64, lifecycleDays int) error
	DeleteStorageBucket(ctx context.Context, id string) error
	BindStorageBucket(ctx context.Context, bucketID, appName, envPrefix string) error
	UnbindStorageBucket(ctx context.Context, bucketID, appName string) error
	ListStorageBucketBindings(ctx context.Context, appName string) ([]store.StorageBucketBinding, error)
}

// BucketSweepService runs a bucket's lifecycle rule on demand.
type BucketSweepService interface {
	Sweep(ctx context.Context, b store.StorageBucket) (objectstore.SweepResult, error)
}

type bucketAppRef struct {
	AppName   string `json:"app_name"`
	EnvPrefix string `json:"env_prefix"`
}

type storageBucketResource struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	TargetID      string         `json:"target_id"`
	Prefix        string         `json:"prefix"`
	Public        bool           `json:"public"`
	PublicURL     string         `json:"public_url,omitempty"`
	QuotaBytes    int64          `json:"quota_bytes"`
	AppRestarted  bool           `json:"app_restarted,omitempty"`
	LifecycleDays int            `json:"lifecycle_days"`
	LastSweptAt   string         `json:"last_swept_at,omitempty"`
	CreatedAt     string         `json:"created_at"`
	Apps          []bucketAppRef `json:"apps"`
	EnvKeys       []string       `json:"env_keys,omitempty"`
}

func bucketResource(b store.StorageBucket, apps []bucketAppRef) storageBucketResource {
	res := storageBucketResource{
		ID: b.ID, Name: b.Name, TargetID: b.TargetID, Prefix: b.Prefix, Public: b.Public, QuotaBytes: b.QuotaBytes,
		LifecycleDays: b.LifecycleDays, LastSweptAt: b.LastSweptAt, CreatedAt: b.CreatedAt, Apps: apps,
	}
	if res.Apps == nil {
		res.Apps = []bucketAppRef{}
	}
	if b.Public {
		res.PublicURL = publicBucketPathPrefix + b.ID + "/"
	}
	return res
}

func (rt *Router) bucketsOrNotImplemented(w http.ResponseWriter) (*StorageDeps, bool) {
	d, ok := rt.storageOrNotImplemented(w)
	if !ok {
		return nil, false
	}
	if d.Buckets == nil {
		writeError(w, http.StatusNotImplemented, "storage buckets are not configured on this control plane")
		return nil, false
	}
	return d, true
}

func bucketAppsByID(bindings []store.StorageBucketBinding) map[string][]bucketAppRef {
	out := map[string][]bucketAppRef{}
	for _, l := range bindings {
		out[l.Bucket.ID] = append(out[l.Bucket.ID], bucketAppRef{AppName: l.AppName, EnvPrefix: l.EnvPrefix})
	}
	return out
}

func bucketEnvKeys(envPrefix string) []string {
	keys := make([]string, 0, 6)
	for _, s := range []string{"ENDPOINT", "BUCKET", "PREFIX", "REGION", "ACCESS_KEY_ID", "SECRET_ACCESS_KEY"} {
		keys = append(keys, envPrefix+"_"+s)
	}
	return keys
}

// handleListStorageBuckets handles GET /api/v1/storage/buckets, optionally filtered with ?app=.
func (rt *Router) handleListStorageBuckets(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.bucketsOrNotImplemented(w)
	if !ok {
		return
	}
	app := r.URL.Query().Get("app")
	if !rt.authorizeRefs(w, r, AbilityRead, notFoundAs(http.StatusNotFound, "app not found"), appRef(app)) {
		return
	}
	bindings, err := d.Buckets.ListStorageBucketBindings(r.Context(), "")
	if err != nil {
		rt.internalError(w, "api: list bucket bindings failed", err)
		return
	}
	apps := bucketAppsByID(bindings)
	buckets, err := d.Buckets.ListStorageBuckets(r.Context())
	if err != nil {
		rt.internalError(w, "api: list buckets failed", err)
		return
	}
	out := make([]storageBucketResource, 0, len(buckets))
	for _, b := range buckets {
		if b.Purpose == store.StorageBucketPurposeAssets || (app != "" && !bucketBoundTo(apps[b.ID], app)) {
			continue
		}
		out = append(out, bucketResource(b, apps[b.ID]))
	}
	writeJSON(w, http.StatusOK, out)
}

func bucketBoundTo(refs []bucketAppRef, app string) bool {
	for _, r := range refs {
		if r.AppName == app {
			return true
		}
	}
	return false
}

type createStorageBucketRequest struct {
	Name          string `json:"name"`
	TargetID      string `json:"target_id"`
	AppName       string `json:"app_name"`
	EnvPrefix     string `json:"env_prefix"`
	Public        bool   `json:"public"`
	QuotaBytes    int64  `json:"quota_bytes"`
	LifecycleDays int    `json:"lifecycle_days"`
}

func validateBucketLimits(quota int64, days int) string {
	if quota < 0 {
		return "quota_bytes must not be negative"
	}
	if days < 0 || days > maxBucketLifecycleDays {
		return fmt.Sprintf("lifecycle_days must be between 0 and %d", maxBucketLifecycleDays)
	}
	return ""
}

// handleCreateStorageBucket handles POST /api/v1/storage/buckets. With app_name it also binds the new bucket.
func (rt *Router) handleCreateStorageBucket(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.bucketsOrNotImplemented(w)
	if !ok {
		return
	}
	var req createStorageBucketRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if !bucketNameRE.MatchString(req.Name) {
		writeError(w, http.StatusBadRequest, "name must be 2 to 63 lowercase letters, digits or dashes")
		return
	}
	if msg := validateBucketLimits(req.QuotaBytes, req.LifecycleDays); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	envPrefix := req.EnvPrefix
	if envPrefix == "" {
		envPrefix = defaultBucketEnvPrefix
	}
	if !bucketEnvPrefixRE.MatchString(envPrefix) {
		writeError(w, http.StatusBadRequest, "env_prefix must be uppercase letters, digits and underscores")
		return
	}
	if _, err := rt.backupTargets.GetBackupTarget(r.Context(), req.TargetID); errors.Is(err, store.ErrBackupTargetNotFound) {
		writeError(w, http.StatusBadRequest, "unknown target_id")
		return
	} else if err != nil {
		rt.internalError(w, "api: create bucket: look up destination failed", err)
		return
	}
	if !rt.authorizeRefs(w, r, AbilityWrite, notFoundAs(http.StatusBadRequest, "unknown app_name"), appRef(req.AppName)) {
		return
	}
	if req.AppName != "" {
		if _, err := rt.apps.GetDesiredService(r.Context(), req.AppName); errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusBadRequest, "unknown app_name")
			return
		} else if err != nil {
			rt.internalError(w, "api: create bucket: look up app failed", err)
			return
		}
	}
	id, err := randomLogArchiveID(storageBucketIDPrefix)
	if err != nil {
		rt.internalError(w, "api: create bucket: mint id failed", err)
		return
	}
	b, err := d.Buckets.CreateStorageBucket(r.Context(), store.StorageBucket{
		ID: id, Name: req.Name, TargetID: req.TargetID, Prefix: objectstore.BucketPrefix(id),
		Public: req.Public, QuotaBytes: req.QuotaBytes, LifecycleDays: req.LifecycleDays,
	})
	if errors.Is(err, store.ErrStorageBucketExists) {
		writeError(w, http.StatusConflict, "a bucket with that name already exists")
		return
	}
	if err != nil {
		rt.internalError(w, "api: create bucket failed", err)
		return
	}
	var apps []bucketAppRef
	if req.AppName != "" {
		if err := d.Buckets.BindStorageBucket(r.Context(), b.ID, req.AppName, envPrefix); err != nil {
			rt.internalError(w, "api: bind new bucket failed", err, slog.String("bucket_id", b.ID))
			return
		}
		apps = []bucketAppRef{{AppName: req.AppName, EnvPrefix: envPrefix}}
	}
	res := bucketResource(b, apps)
	if len(apps) > 0 {
		res.EnvKeys = bucketEnvKeys(envPrefix)
	}
	writeJSON(w, http.StatusCreated, res)
}

func (rt *Router) loadBucket(w http.ResponseWriter, r *http.Request) (*StorageDeps, store.StorageBucket, bool) {
	d, ok := rt.bucketsOrNotImplemented(w)
	if !ok {
		return nil, store.StorageBucket{}, false
	}
	b, err := d.Buckets.GetStorageBucket(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrStorageBucketNotFound) || (err == nil && b.Purpose == store.StorageBucketPurposeAssets) {
		writeError(w, http.StatusNotFound, "bucket not found")
		return nil, store.StorageBucket{}, false
	}
	if err != nil {
		rt.internalError(w, "api: get bucket failed", err)
		return nil, store.StorageBucket{}, false
	}
	return d, b, true
}

func (rt *Router) bucketAppRefs(ctx context.Context, d *StorageDeps, id string) ([]bucketAppRef, error) {
	bindings, err := d.Buckets.ListStorageBucketBindings(ctx, "")
	if err != nil {
		return nil, err
	}
	return bucketAppsByID(bindings)[id], nil
}

// handleGetStorageBucket handles GET /api/v1/storage/buckets/{id}.
func (rt *Router) handleGetStorageBucket(w http.ResponseWriter, r *http.Request) {
	d, b, ok := rt.loadBucket(w, r)
	if !ok {
		return
	}
	apps, err := rt.bucketAppRefs(r.Context(), d, b.ID)
	if err != nil {
		rt.internalError(w, "api: get bucket bindings failed", err)
		return
	}
	writeJSON(w, http.StatusOK, bucketResource(b, apps))
}

type updateStorageBucketRequest struct {
	Public        *bool  `json:"public"`
	QuotaBytes    *int64 `json:"quota_bytes"`
	LifecycleDays *int   `json:"lifecycle_days"`
}

// handleUpdateStorageBucket handles PATCH /api/v1/storage/buckets/{id}: only the fields sent change.
func (rt *Router) handleUpdateStorageBucket(w http.ResponseWriter, r *http.Request) {
	d, b, ok := rt.loadBucket(w, r)
	if !ok {
		return
	}
	var req updateStorageBucketRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if req.Public != nil {
		b.Public = *req.Public
	}
	if req.QuotaBytes != nil {
		b.QuotaBytes = *req.QuotaBytes
	}
	if req.LifecycleDays != nil {
		b.LifecycleDays = *req.LifecycleDays
	}
	if msg := validateBucketLimits(b.QuotaBytes, b.LifecycleDays); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if err := d.Buckets.UpdateStorageBucketSettings(r.Context(), b.ID, b.Public, b.QuotaBytes, b.LifecycleDays); err != nil {
		rt.internalError(w, "api: update bucket failed", err)
		return
	}
	apps, err := rt.bucketAppRefs(r.Context(), d, b.ID)
	if err != nil {
		rt.internalError(w, "api: get bucket bindings failed", err)
		return
	}
	writeJSON(w, http.StatusOK, bucketResource(b, apps))
}

// handleDeleteStorageBucket handles DELETE /api/v1/storage/buckets/{id}; ?purge=true also deletes its objects.
func (rt *Router) handleDeleteStorageBucket(w http.ResponseWriter, r *http.Request) {
	d, b, ok := rt.loadBucket(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("purge") == "true" {
		ctx, cancel := context.WithTimeout(r.Context(), bucketOpTimeout)
		defer cancel()
		client, _, err := d.Clients.Client(ctx, b.TargetID)
		if err != nil {
			rt.internalError(w, "api: delete bucket: resolve client failed", err)
			return
		}
		if _, err := client.DeletePrefix(ctx, b.Prefix); err != nil {
			rt.internalError(w, "api: delete bucket: purge failed", err, slog.String("bucket_id", b.ID))
			return
		}
	}
	if err := d.Buckets.DeleteStorageBucket(r.Context(), b.ID); err != nil {
		rt.internalError(w, "api: delete bucket failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type bindStorageBucketRequest struct {
	EnvPrefix string `json:"env_prefix"`
}

// handleListAppBuckets handles GET /api/v1/apps/{name}/buckets.
func (rt *Router) handleListAppBuckets(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.bucketsOrNotImplemented(w)
	if !ok {
		return
	}
	bindings, err := d.Buckets.ListStorageBucketBindings(r.Context(), r.PathValue("name"))
	if err != nil {
		rt.internalError(w, "api: list app buckets failed", err)
		return
	}
	out := make([]storageBucketResource, 0, len(bindings))
	for _, l := range bindings {
		res := bucketResource(l.Bucket, []bucketAppRef{{AppName: l.AppName, EnvPrefix: l.EnvPrefix}})
		res.EnvKeys = bucketEnvKeys(l.EnvPrefix)
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleBindAppBucket handles PUT /api/v1/apps/{name}/buckets/{id}: the next container start gets the bucket's env vars.
func (rt *Router) handleBindAppBucket(w http.ResponseWriter, r *http.Request) {
	d, b, ok := rt.loadBucket(w, r)
	if !ok {
		return
	}
	app := r.PathValue("name")
	var req bindStorageBucketRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	if req.EnvPrefix == "" {
		req.EnvPrefix = defaultBucketEnvPrefix
	}
	if !bucketEnvPrefixRE.MatchString(req.EnvPrefix) {
		writeError(w, http.StatusBadRequest, "env_prefix must be uppercase letters, digits and underscores")
		return
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), app); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: bind bucket: look up app failed", err)
		return
	}
	err := d.Buckets.BindStorageBucket(r.Context(), b.ID, app, req.EnvPrefix)
	if errors.Is(err, store.ErrStorageBucketExists) {
		writeError(w, http.StatusConflict, "this app already uses that env prefix for another bucket")
		return
	}
	if err != nil {
		rt.internalError(w, "api: bind bucket failed", err)
		return
	}
	res := bucketResource(b, []bucketAppRef{{AppName: app, EnvPrefix: req.EnvPrefix}})
	res.EnvKeys = bucketEnvKeys(req.EnvPrefix)
	res.AppRestarted = rt.restartAppForBucketChange(r.Context(), app)
	writeJSON(w, http.StatusOK, res)
}

// handleUnbindAppBucket handles DELETE /api/v1/apps/{name}/buckets/{id}.
func (rt *Router) handleUnbindAppBucket(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.bucketsOrNotImplemented(w)
	if !ok {
		return
	}
	if err := d.Buckets.UnbindStorageBucket(r.Context(), r.PathValue("id"), r.PathValue("name")); err != nil {
		rt.internalError(w, "api: unbind bucket failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"app_restarted": rt.restartAppForBucketChange(r.Context(), r.PathValue("name"))})
}

func (rt *Router) bucketClient(ctx context.Context, w http.ResponseWriter, d *StorageDeps, b store.StorageBucket) (*objectstore.Client, bool) {
	client, _, err := d.Clients.Client(ctx, b.TargetID)
	if err != nil {
		rt.internalError(w, "api: bucket: resolve destination failed", err, slog.String("bucket_id", b.ID))
		return nil, false
	}
	return client, true
}

type bucketObjectsResource struct {
	Objects []objectstore.Object `json:"objects"`
	Next    string               `json:"next,omitempty"`
}

// handleListBucketObjects handles GET /api/v1/storage/buckets/{id}/objects?prefix=&token=&limit=. Keys are relative to the bucket.
func (rt *Router) handleListBucketObjects(w http.ResponseWriter, r *http.Request) {
	d, b, ok := rt.loadBucket(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), bucketOpTimeout)
	defer cancel()
	client, ok := rt.bucketClient(ctx, w, d, b)
	if !ok {
		return
	}
	limit := int32(bucketListLimit)
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v < bucketListLimit {
		limit = int32(v) //nolint:gosec // v is bounded above by bucketListLimit
	}
	page, err := client.List(ctx, b.Prefix+r.URL.Query().Get("prefix"), r.URL.Query().Get("token"), limit)
	if err != nil {
		rt.internalError(w, "api: list bucket objects failed", err, slog.String("bucket_id", b.ID))
		return
	}
	for i := range page.Objects {
		page.Objects[i].Key = page.Objects[i].Key[len(b.Prefix):]
	}
	writeJSON(w, http.StatusOK, bucketObjectsResource{Objects: page.Objects, Next: page.Next})
}

type bucketUsageResource struct {
	Bytes      int64 `json:"bytes"`
	Objects    int64 `json:"objects"`
	QuotaBytes int64 `json:"quota_bytes"`
}

// handleBucketUsage handles GET /api/v1/storage/buckets/{id}/usage.
func (rt *Router) handleBucketUsage(w http.ResponseWriter, r *http.Request) {
	d, b, ok := rt.loadBucket(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), bucketOpTimeout)
	defer cancel()
	client, ok := rt.bucketClient(ctx, w, d, b)
	if !ok {
		return
	}
	bytes, objects, err := client.Usage(ctx, b.Prefix)
	if err != nil {
		rt.internalError(w, "api: bucket usage failed", err, slog.String("bucket_id", b.ID))
		return
	}
	writeJSON(w, http.StatusOK, bucketUsageResource{Bytes: bytes, Objects: objects, QuotaBytes: b.QuotaBytes})
}

// bucketHasRoom reports whether adding extra bytes keeps b within its quota.
func (rt *Router) bucketHasRoom(ctx context.Context, client *objectstore.Client, b store.StorageBucket, extra int64) (bool, error) {
	if b.QuotaBytes <= 0 {
		return true, nil
	}
	used, _, err := client.Usage(ctx, b.Prefix)
	if err != nil {
		return false, err
	}
	return used+extra <= b.QuotaBytes, nil
}

// handleUploadBucketObject handles PUT /api/v1/storage/buckets/{id}/objects?key=: the raw body becomes the object.
func (rt *Router) handleUploadBucketObject(w http.ResponseWriter, r *http.Request) {
	d, b, ok := rt.loadBucket(w, r)
	if !ok {
		return
	}
	key, err := objectstore.ScopedKey(b.Prefix, r.URL.Query().Get("key"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid key")
		return
	}
	tmp, err := os.CreateTemp("", "bucket-upload-*")
	if err != nil {
		rt.internalError(w, "api: bucket upload: temp file failed", err)
		return
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	size, err := io.Copy(tmp, r.Body)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "file is too large for a console upload, use a presigned URL")
			return
		}
		writeError(w, http.StatusBadRequest, "could not read upload")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), bucketOpTimeout)
	defer cancel()
	client, ok := rt.bucketClient(ctx, w, d, b)
	if !ok {
		return
	}
	room, err := rt.bucketHasRoom(ctx, client, b, size)
	if err != nil {
		rt.internalError(w, "api: bucket upload: quota check failed", err, slog.String("bucket_id", b.ID))
		return
	}
	if !room {
		writeError(w, http.StatusRequestEntityTooLarge, "bucket quota exceeded")
		return
	}
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = mime.TypeByExtension(path.Ext(key))
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		rt.internalError(w, "api: bucket upload: rewind failed", err)
		return
	}
	if err := client.Put(ctx, key, tmp, contentType, ""); err != nil {
		rt.internalError(w, "api: bucket upload failed", err, slog.String("bucket_id", b.ID))
		return
	}
	writeJSON(w, http.StatusCreated, objectstore.Object{Key: key[len(b.Prefix):], Size: size, LastModified: time.Now().UTC()})
}

func setObjectHeaders(w http.ResponseWriter, key string, length int64, attachment bool) {
	ct := mime.TypeByExtension(path.Ext(key))
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", bucketObjectSandboxPolicy)
	if attachment {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(key)}))
	}
	if length > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	}
}

func (rt *Router) streamBucketObject(w http.ResponseWriter, r *http.Request, d *StorageDeps, b store.StorageBucket, rel string, attachment bool) {
	public := !attachment
	key, err := objectstore.ScopedKey(b.Prefix, rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid key")
		return
	}
	client, ok := rt.bucketClient(r.Context(), w, d, b)
	if !ok {
		return
	}
	body, length, err := client.Get(r.Context(), key)
	if err != nil {
		writeError(w, http.StatusNotFound, "object not found")
		return
	}
	defer func() { _ = body.Close() }()
	if public {
		maxBytes := envPositiveInt64(envPublicBucketMaxBytes, defaultPublicBucketMaxBytes)
		if length > maxBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "object is too large for the public route, use a presigned URL")
			return
		}
		body = struct {
			io.Reader
			io.Closer
		}{io.LimitReader(body, maxBytes), body}
		attachment = isActiveContentType(mime.TypeByExtension(path.Ext(key)))
	}
	setObjectHeaders(w, key, length, attachment)
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}

// handleDownloadBucketObject handles GET /api/v1/storage/buckets/{id}/objects/download?key=.
func (rt *Router) handleDownloadBucketObject(w http.ResponseWriter, r *http.Request) {
	d, b, ok := rt.loadBucket(w, r)
	if !ok {
		return
	}
	rt.streamBucketObject(w, r, d, b, r.URL.Query().Get("key"), true)
}

// handleDeleteBucketObject handles DELETE /api/v1/storage/buckets/{id}/objects?key=.
func (rt *Router) handleDeleteBucketObject(w http.ResponseWriter, r *http.Request) {
	d, b, ok := rt.loadBucket(w, r)
	if !ok {
		return
	}
	key, err := objectstore.ScopedKey(b.Prefix, r.URL.Query().Get("key"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid key")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), bucketOpTimeout)
	defer cancel()
	client, ok := rt.bucketClient(ctx, w, d, b)
	if !ok {
		return
	}
	if err := client.Delete(ctx, key); err != nil {
		rt.internalError(w, "api: bucket delete object failed", err, slog.String("bucket_id", b.ID))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type presignRequest struct {
	Key            string `json:"key"`
	Method         string `json:"method"`
	ExpiresSeconds int    `json:"expires_seconds"`
}

type presignResource struct {
	URL       string `json:"url"`
	Method    string `json:"method"`
	ExpiresAt string `json:"expires_at"`
}

// handlePresignBucketObject handles POST /api/v1/storage/buckets/{id}/presign.
func (rt *Router) handlePresignBucketObject(w http.ResponseWriter, r *http.Request) {
	d, b, ok := rt.loadBucket(w, r)
	if !ok {
		return
	}
	var req presignRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if req.Method == "" {
		req.Method = http.MethodGet
	}
	if req.Method != http.MethodGet && req.Method != http.MethodPut {
		writeError(w, http.StatusBadRequest, "method must be GET or PUT")
		return
	}
	if req.ExpiresSeconds <= 0 {
		req.ExpiresSeconds = defaultPresignSeconds
	}
	if req.ExpiresSeconds > maxPresignSeconds {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("expires_seconds must be at most %d", maxPresignSeconds))
		return
	}
	key, err := objectstore.ScopedKey(b.Prefix, req.Key)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid key")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), bucketOpTimeout)
	defer cancel()
	client, ok := rt.bucketClient(ctx, w, d, b)
	if !ok {
		return
	}
	ttl := time.Duration(req.ExpiresSeconds) * time.Second
	var url string
	if req.Method == http.MethodPut {
		room, err := rt.bucketHasRoom(ctx, client, b, 0)
		if err != nil {
			rt.internalError(w, "api: presign: quota check failed", err, slog.String("bucket_id", b.ID))
			return
		}
		if !room {
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("bucket quota of %d bytes is full: delete objects or raise quota_bytes before uploading", b.QuotaBytes))
			return
		}
		url, err = client.PresignPut(ctx, key, ttl)
		if err != nil {
			rt.internalError(w, "api: presign put failed", err, slog.String("bucket_id", b.ID))
			return
		}
	} else {
		url, err = client.PresignGet(ctx, key, ttl)
		if err != nil {
			rt.internalError(w, "api: presign get failed", err, slog.String("bucket_id", b.ID))
			return
		}
	}
	writeJSON(w, http.StatusOK, presignResource{URL: url, Method: req.Method, ExpiresAt: time.Now().Add(ttl).UTC().Format(time.RFC3339)})
}

// handleSweepStorageBucket handles POST /api/v1/storage/buckets/{id}/lifecycle/run.
func (rt *Router) handleSweepStorageBucket(w http.ResponseWriter, r *http.Request) {
	d, b, ok := rt.loadBucket(w, r)
	if !ok {
		return
	}
	if d.BucketSweeper == nil {
		writeError(w, http.StatusNotImplemented, "bucket lifecycle is not configured on this control plane")
		return
	}
	if b.LifecycleDays <= 0 {
		writeError(w, http.StatusBadRequest, "this bucket has no lifecycle rule")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	res, err := d.BucketSweeper.Sweep(ctx, b)
	if err != nil {
		rt.internalError(w, "api: bucket sweep failed", err, slog.String("bucket_id", b.ID))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handlePublicBucketObject handles GET /api/v1/public/buckets/{id}/{key...}. Unauthenticated, and only
// answers for buckets whose public flag is on; every other bucket looks like a missing one.
func (rt *Router) handlePublicBucketObject(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.bucketsOrNotImplemented(w)
	if !ok {
		return
	}
	if allowed, retry := rt.publicBucketAllowed(r); !allowed {
		writeRateLimited(w, retry)
		return
	}
	b, err := d.Buckets.GetStorageBucket(r.Context(), r.PathValue("id"))
	if err != nil || !b.Public || b.Purpose == store.StorageBucketPurposeAssets {
		writeError(w, http.StatusNotFound, "object not found")
		return
	}
	rt.streamBucketObject(w, r, d, b, r.PathValue("key"), false)
}

func (rt *Router) publicBucketAllowed(r *http.Request) (bool, time.Duration) {
	rt.publicBucketLimiterOnce.Do(func() {
		rt.publicBucketLimiter = newAPIRateLimiter(envPositiveInt(envPublicBucketRatePerMin, defaultPublicBucketRatePerMin))
	})
	return rt.publicBucketLimiter.allow(clientIP(r))
}

func envPositiveInt64(name string, def int64) int64 {
	if v, err := strconv.ParseInt(os.Getenv(name), 10, 64); err == nil && v > 0 {
		return v
	}
	return def
}

// isActiveContentType reports types a browser would execute or render as a page, which the public
// route never serves inline.
func isActiveContentType(ct string) bool {
	base, _, _ := strings.Cut(ct, ";")
	switch strings.ToLower(strings.TrimSpace(base)) {
	case "text/html", "application/xhtml+xml", "image/svg+xml", "text/xml", "application/xml",
		"text/javascript", "application/javascript", "application/x-javascript":
		return true
	}
	return false
}
