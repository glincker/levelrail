package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envAssetsUploadMaxBytes    = "APP_ASSETS_UPLOAD_MAX_BYTES"
	envAssetsPresignMaxTTL     = "APP_ASSETS_PRESIGN_MAX_TTL"
	envAssetsUsageMaxObjects   = "APP_ASSETS_USAGE_MAX_OBJECTS"
	envAssetsExposureTimeout   = "APP_ASSETS_EXPOSURE_TIMEOUT"
	defaultAssetsUploadMax     = int64(1) << 30
	defaultAssetsPresignTTL    = time.Hour
	defaultAssetsUsageMaxObjs  = int64(100000)
	defaultAssetsExposureTO    = 10 * time.Second
	resourcePrefixAssetBucket  = "asset_bucket:"
	resourcePrefixProjectScope = "project:"

	auditAssetExposureChanged = "asset_bucket.exposure_changed"
	auditAssetBucketCreated   = "asset_bucket.created"
	auditAssetBucketDeleted   = "asset_bucket.deleted"
	assetListLimit            = 200
	maxAssetReferrerHosts     = 50
)

// AssetBucketStore persists asset buckets and the asset index (ADR 044).
type AssetBucketStore interface {
	CreateAssetBucket(ctx context.Context, b store.StorageBucket, s store.AssetBucketSettings) (store.AssetBucket, error)
	GetAssetBucket(ctx context.Context, id string) (store.AssetBucket, error)
	ListAssetBuckets(ctx context.Context, projectID string) ([]store.AssetBucket, error)
	UpdateAssetBucketSettings(ctx context.Context, s store.AssetBucketSettings) error
	UpdateStorageBucketSettings(ctx context.Context, id string, public bool, quotaBytes int64, lifecycleDays int) error
	DeleteStorageBucket(ctx context.Context, id string) error
	UpsertAsset(ctx context.Context, a store.Asset) (store.Asset, error)
	GetAsset(ctx context.Context, bucketID, key string) (store.Asset, error)
	DeleteAsset(ctx context.Context, bucketID, key string) error
}

// assetsDeps is what every asset route needs.
type assetsDeps struct {
	Assets  AssetBucketStore
	Clients ObjectClientResolver
	Options StorageOptionsStore
}

func (rt *Router) assetsOrNotImplemented(w http.ResponseWriter) (assetsDeps, bool) {
	d, ok := rt.storageOrNotImplemented(w)
	if !ok {
		return assetsDeps{}, false
	}
	if d.AssetBuckets == nil || d.Clients == nil {
		writeError(w, http.StatusNotImplemented, "assets are not configured on this control plane")
		return assetsDeps{}, false
	}
	return assetsDeps{Assets: d.AssetBuckets, Clients: d.Clients, Options: d.Options}, true
}

// assetBucketResource never carries credentials; they stay with the storage destination.
type assetBucketResource struct {
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

func toAssetBucketResource(ab store.AssetBucket, preset string) assetBucketResource {
	allow := ab.Settings.ReferrerAllow
	if allow == nil {
		allow = []string{}
	}
	return assetBucketResource{
		ID: ab.Bucket.ID, Name: ab.Bucket.Name, TargetID: ab.Bucket.TargetID, Preset: preset, Prefix: ab.Bucket.Prefix,
		ProjectID: ab.Settings.ProjectID, Visibility: ab.Settings.Visibility, ServingMode: ab.Settings.ServingMode,
		PublicBaseURL: ab.Settings.PublicBaseURL, CDNBaseURL: ab.Settings.CDNBaseURL, ReferrerAllow: allow,
		RefuseNoReferer: ab.Settings.RefuseNoReferer, QuotaBytes: ab.Bucket.QuotaBytes,
		CreatedAt: ab.Bucket.CreatedAt, UpdatedAt: ab.Settings.UpdatedAt,
	}
}

func (rt *Router) destinationPreset(ctx context.Context, d assetsDeps, targetID string) string {
	if d.Options != nil {
		if o, ok, err := d.Options.GetStorageOptions(ctx, targetID); err == nil && ok && o.Preset != "" {
			return o.Preset
		}
	}
	if t, err := rt.backupTargets.GetBackupTarget(ctx, targetID); err == nil {
		return objectstore.PresetForProvider(t.Provider)
	}
	return objectstore.PresetCustom
}

// assetBucketAllowed applies IAM to asset_bucket:<id>, with project:<id> as a covering scope.
func (rt *Router) assetBucketAllowed(ctx context.Context, abilities []string, policies []store.Policy, ability, id string) bool {
	var scope []string
	if d, ok := rt.assetStoreOrNil(); ok {
		if ab, err := d.GetAssetBucket(ctx, id); err == nil && ab.Settings.ProjectID != "" {
			scope = append(scope, resourcePrefixProjectScope+ab.Settings.ProjectID)
		}
	}
	return authorizeResource(abilities, policies, ability, resourcePrefixAssetBucket+id, scope...)
}

func (rt *Router) assetStoreOrNil() (AssetBucketStore, bool) {
	if rt.storage == nil || rt.storage.AssetBuckets == nil {
		return nil, false
	}
	return rt.storage.AssetBuckets, true
}

// requireAssetBucket gates a route on the ability for the asset bucket in the path. An explicit Deny on the
// bucket or its project wins; an Allow scoped to either can grant what the token lacks globally.
func (rt *Router) requireAssetBucket(required string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		decide := func(ctx context.Context, principalType, principalID string, abilities []string) (bool, error) {
			policies, err := rt.policies.ListPoliciesForPrincipal(ctx, principalType, principalID)
			if err != nil {
				return false, err
			}
			return rt.assetBucketAllowed(ctx, abilities, policies, required, id), nil
		}
		rt.requireAbilityDecided(required, decide, next)(w, r)
	}
}

func (rt *Router) loadAssetBucket(w http.ResponseWriter, r *http.Request) (assetsDeps, store.AssetBucket, bool) {
	d, ok := rt.assetsOrNotImplemented(w)
	if !ok {
		return assetsDeps{}, store.AssetBucket{}, false
	}
	ab, err := d.Assets.GetAssetBucket(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrAssetBucketNotFound) {
		writeError(w, http.StatusNotFound, "asset bucket not found")
		return assetsDeps{}, store.AssetBucket{}, false
	}
	if err != nil {
		rt.internalError(w, "api: get asset bucket failed", err)
		return assetsDeps{}, store.AssetBucket{}, false
	}
	return d, ab, true
}

func (rt *Router) assetClient(ctx context.Context, w http.ResponseWriter, d assetsDeps, ab store.AssetBucket) (*objectstore.Client, bool) {
	client, _, err := d.Clients.Client(ctx, ab.Bucket.TargetID)
	if err != nil {
		rt.internalError(w, "api: asset bucket: resolve destination failed", err, slog.String("bucket_id", ab.Bucket.ID))
		return nil, false
	}
	return client, true
}

// handleListAssetBuckets handles GET /api/v1/assets/buckets, optionally filtered with ?project=.
func (rt *Router) handleListAssetBuckets(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.assetsOrNotImplemented(w)
	if !ok {
		return
	}
	project := r.URL.Query().Get("project")
	if !rt.authorizeRefs(w, r, AbilityRead, notFoundAs(http.StatusNotFound, "project not found"), projectRef(project)) {
		return
	}
	buckets, err := d.Assets.ListAssetBuckets(r.Context(), project)
	if err != nil {
		rt.internalError(w, "api: list asset buckets failed", err)
		return
	}
	allowed, err := rt.authorizeRefPredicate(r, AbilityRead)
	if err != nil {
		rt.internalError(w, "api: list asset buckets: authorize", err)
		return
	}
	out := make([]assetBucketResource, 0, len(buckets))
	for _, ab := range buckets {
		ok, err := allowed(assetBucketRef(ab.Bucket.ID))
		if err != nil {
			rt.internalError(w, "api: list asset buckets: authorize bucket", err)
			return
		}
		if ok {
			out = append(out, toAssetBucketResource(ab, rt.destinationPreset(r.Context(), d, ab.Bucket.TargetID)))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type createAssetBucketRequest struct {
	Name           string   `json:"name"`
	TargetID       string   `json:"target_id"`
	Prefix         string   `json:"prefix"`
	ProjectID      string   `json:"project_id"`
	Visibility     string   `json:"visibility"`
	ConfirmName    string   `json:"confirm_name"`
	ServingMode    string   `json:"serving_mode"`
	PublicBaseURL  string   `json:"public_base_url"`
	CDNBaseURL     string   `json:"cdn_base_url"`
	ReferrerAllow  []string `json:"referrer_allow"`
	AllowNoReferer *bool    `json:"allow_no_referer"`
	QuotaBytes     int64    `json:"quota_bytes"`
}

// handleCreateAssetBucket handles POST /api/v1/assets/buckets. The bucket is private and Direct unless told otherwise.
func (rt *Router) handleCreateAssetBucket(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.assetsOrNotImplemented(w)
	if !ok {
		return
	}
	var req createAssetBucketRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if !bucketNameRE.MatchString(req.Name) {
		writeError(w, http.StatusBadRequest, "name must be 2 to 63 lowercase letters, digits or dashes")
		return
	}
	if req.QuotaBytes < 0 {
		writeError(w, http.StatusBadRequest, "quota_bytes must not be negative")
		return
	}
	prefix, msg := normalizeAssetPrefix(req.Prefix)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	s := store.AssetBucketSettings{
		ProjectID: req.ProjectID, Visibility: req.Visibility, ServingMode: req.ServingMode,
		PublicBaseURL: req.PublicBaseURL, CDNBaseURL: req.CDNBaseURL, ReferrerAllow: req.ReferrerAllow,
	}
	if msg := validateAssetSettings(&s); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	s.RefuseNoReferer = len(s.ReferrerAllow) > 0 && (req.AllowNoReferer == nil || !*req.AllowNoReferer)
	if s.Visibility == store.AssetVisibilityPublic && req.ConfirmName != req.Name {
		writeCodedError(w, http.StatusBadRequest, errCodeAssetConfirm, "making a bucket public needs confirm_name set to the bucket name")
		return
	}
	if _, err := rt.backupTargets.GetBackupTarget(r.Context(), req.TargetID); errors.Is(err, store.ErrBackupTargetNotFound) {
		writeError(w, http.StatusBadRequest, "unknown target_id")
		return
	} else if err != nil {
		rt.internalError(w, "api: create asset bucket: look up destination failed", err)
		return
	}
	if !rt.authorizeRefs(w, r, AbilityWrite, notFoundAs(http.StatusBadRequest, "unknown project_id"), projectRef(s.ProjectID)) {
		return
	}
	if s.ProjectID != "" {
		if _, err := rt.projects.GetProject(r.Context(), s.ProjectID); errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusBadRequest, "unknown project_id")
			return
		} else if err != nil {
			rt.internalError(w, "api: create asset bucket: look up project failed", err)
			return
		}
	}
	id, err := randomLogArchiveID(storageBucketIDPrefix)
	if err != nil {
		rt.internalError(w, "api: create asset bucket: mint id failed", err)
		return
	}
	ab, err := d.Assets.CreateAssetBucket(r.Context(), store.StorageBucket{
		ID: id, Name: req.Name, TargetID: req.TargetID, Prefix: prefix, Public: s.Visibility == store.AssetVisibilityPublic, QuotaBytes: req.QuotaBytes,
	}, s)
	if errors.Is(err, store.ErrStorageBucketExists) {
		writeError(w, http.StatusConflict, "a bucket with that name or prefix already exists")
		return
	}
	if err != nil {
		rt.internalError(w, "api: create asset bucket failed", err)
		return
	}
	rt.auditAsset(r, auditAssetBucketCreated, ab.Bucket.ID, map[string]any{
		"name": ab.Bucket.Name, "visibility": s.Visibility, "serving_mode": s.ServingMode,
	})
	writeJSON(w, http.StatusCreated, toAssetBucketResource(ab, rt.destinationPreset(r.Context(), d, ab.Bucket.TargetID)))
}

// handleGetAssetBucket handles GET /api/v1/assets/buckets/{id}.
func (rt *Router) handleGetAssetBucket(w http.ResponseWriter, r *http.Request) {
	d, ab, ok := rt.loadAssetBucket(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toAssetBucketResource(ab, rt.destinationPreset(r.Context(), d, ab.Bucket.TargetID)))
}

type updateAssetBucketRequest struct {
	ProjectID      *string   `json:"project_id"`
	Visibility     *string   `json:"visibility"`
	ConfirmName    string    `json:"confirm_name"`
	ServingMode    *string   `json:"serving_mode"`
	PublicBaseURL  *string   `json:"public_base_url"`
	CDNBaseURL     *string   `json:"cdn_base_url"`
	ReferrerAllow  *[]string `json:"referrer_allow"`
	AllowNoReferer *bool     `json:"allow_no_referer"`
	QuotaBytes     *int64    `json:"quota_bytes"`
}

const errCodeAssetConfirm = "asset_confirm_required"

// handleUpdateAssetBucket handles PATCH /api/v1/assets/buckets/{id}: only the fields sent change.
func (rt *Router) handleUpdateAssetBucket(w http.ResponseWriter, r *http.Request) {
	d, ab, ok := rt.loadAssetBucket(w, r)
	if !ok {
		return
	}
	var req updateAssetBucketRequest
	if !decodeReq(w, r, &req) {
		return
	}
	before := ab.Settings
	s := ab.Settings
	if req.ProjectID != nil {
		if !rt.authorizeRefs(w, r, AbilityWrite, notFoundAs(http.StatusBadRequest, "unknown project_id"), projectRef(*req.ProjectID)) {
			return
		}
		if *req.ProjectID != "" {
			if _, err := rt.projects.GetProject(r.Context(), *req.ProjectID); errors.Is(err, store.ErrProjectNotFound) {
				writeError(w, http.StatusBadRequest, "unknown project_id")
				return
			} else if err != nil {
				rt.internalError(w, "api: update asset bucket: look up project failed", err)
				return
			}
		}
		s.ProjectID = *req.ProjectID
	}
	if req.Visibility != nil {
		s.Visibility = *req.Visibility
	}
	if req.ServingMode != nil {
		s.ServingMode = *req.ServingMode
	}
	if req.PublicBaseURL != nil {
		s.PublicBaseURL = *req.PublicBaseURL
	}
	if req.CDNBaseURL != nil {
		s.CDNBaseURL = *req.CDNBaseURL
	}
	if req.ReferrerAllow != nil {
		s.ReferrerAllow = *req.ReferrerAllow
	}
	if msg := validateAssetSettings(&s); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	switch {
	case req.AllowNoReferer != nil:
		s.RefuseNoReferer = len(s.ReferrerAllow) > 0 && !*req.AllowNoReferer
	case len(before.ReferrerAllow) == 0 && len(s.ReferrerAllow) > 0:
		s.RefuseNoReferer = true
	case len(s.ReferrerAllow) == 0:
		s.RefuseNoReferer = false
	}
	if s.Visibility == store.AssetVisibilityPublic && before.Visibility != store.AssetVisibilityPublic && req.ConfirmName != ab.Bucket.Name {
		writeCodedError(w, http.StatusBadRequest, errCodeAssetConfirm, "making a bucket public needs confirm_name set to the bucket name")
		return
	}
	quota := ab.Bucket.QuotaBytes
	if req.QuotaBytes != nil {
		if *req.QuotaBytes < 0 {
			writeError(w, http.StatusBadRequest, "quota_bytes must not be negative")
			return
		}
		quota = *req.QuotaBytes
	}
	if err := d.Assets.UpdateAssetBucketSettings(r.Context(), s); err != nil {
		rt.internalError(w, "api: update asset bucket failed", err, slog.String("bucket_id", ab.Bucket.ID))
		return
	}
	if quota != ab.Bucket.QuotaBytes {
		if err := d.Assets.UpdateStorageBucketSettings(r.Context(), ab.Bucket.ID, s.Visibility == store.AssetVisibilityPublic, quota, ab.Bucket.LifecycleDays); err != nil {
			rt.internalError(w, "api: update asset bucket quota failed", err, slog.String("bucket_id", ab.Bucket.ID))
			return
		}
	}
	if exposureChanged(before, s) {
		rt.auditAsset(r, auditAssetExposureChanged, ab.Bucket.ID, map[string]any{
			"name": ab.Bucket.Name, "old_visibility": before.Visibility, "new_visibility": s.Visibility,
			"old_serving_mode": before.ServingMode, "new_serving_mode": s.ServingMode,
			"base_urls_changed": before.PublicBaseURL != s.PublicBaseURL || before.CDNBaseURL != s.CDNBaseURL,
		})
	}
	updated, err := d.Assets.GetAssetBucket(r.Context(), ab.Bucket.ID)
	if err != nil {
		rt.internalError(w, "api: update asset bucket: reload failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toAssetBucketResource(updated, rt.destinationPreset(r.Context(), d, updated.Bucket.TargetID)))
}

func exposureChanged(a, b store.AssetBucketSettings) bool {
	return a.Visibility != b.Visibility || a.ServingMode != b.ServingMode || a.PublicBaseURL != b.PublicBaseURL || a.CDNBaseURL != b.CDNBaseURL
}

// handleDeleteAssetBucket handles DELETE /api/v1/assets/buckets/{id}. It removes the connection row only:
// objects stay at the provider.
func (rt *Router) handleDeleteAssetBucket(w http.ResponseWriter, r *http.Request) {
	d, ab, ok := rt.loadAssetBucket(w, r)
	if !ok {
		return
	}
	if err := d.Assets.DeleteStorageBucket(r.Context(), ab.Bucket.ID); err != nil {
		rt.internalError(w, "api: delete asset bucket failed", err, slog.String("bucket_id", ab.Bucket.ID))
		return
	}
	rt.auditAsset(r, auditAssetBucketDeleted, ab.Bucket.ID, map[string]any{"name": ab.Bucket.Name})
	w.WriteHeader(http.StatusNoContent)
}

// normalizeAssetPrefix returns the provider key prefix: empty (whole provider bucket) or a relative path ending in a slash.
func normalizeAssetPrefix(p string) (string, string) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", ""
	}
	if strings.HasPrefix(p, "/") || strings.Contains(p, "..") || strings.ContainsRune(p, 0) || len(p) > 512 {
		return "", "prefix must be a relative path without .."
	}
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p, ""
}

// validateAssetSettings normalizes the settings in place and returns a user-facing message for the first problem.
func validateAssetSettings(s *store.AssetBucketSettings) string {
	if s.Visibility == "" {
		s.Visibility = store.AssetVisibilityPrivate
	}
	if s.ServingMode == "" {
		s.ServingMode = store.AssetServingDirect
	}
	if !slices.Contains([]string{store.AssetVisibilityPrivate, store.AssetVisibilityPublic}, s.Visibility) {
		return "visibility must be private or public"
	}
	switch s.ServingMode {
	case store.AssetServingDirect, store.AssetServingCDN:
	case store.AssetServingProxied:
		return "proxied serving is not available yet: use direct or cdn"
	default:
		return "serving_mode must be direct or cdn"
	}
	var msg string
	if s.PublicBaseURL, msg = normalizeBaseURL("public_base_url", s.PublicBaseURL); msg != "" {
		return msg
	}
	if s.CDNBaseURL, msg = normalizeBaseURL("cdn_base_url", s.CDNBaseURL); msg != "" {
		return msg
	}
	if s.ServingMode == store.AssetServingCDN && s.CDNBaseURL == "" {
		return "cdn serving needs cdn_base_url"
	}
	if len(s.ReferrerAllow) > maxAssetReferrerHosts {
		return "referrer_allow has too many hosts"
	}
	hosts := make([]string, 0, len(s.ReferrerAllow))
	for _, h := range s.ReferrerAllow {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" || strings.ContainsAny(h, "/ \t\r\n") {
			return "referrer_allow entries must be bare hosts such as app.example.com"
		}
		hosts = append(hosts, h)
	}
	s.ReferrerAllow = hosts
	return ""
}

func normalizeBaseURL(field, v string) (string, string) {
	v = strings.TrimRight(strings.TrimSpace(v), "/")
	if v == "" {
		return "", ""
	}
	if err := objectstore.ValidateEndpoint(v); err != nil {
		return "", field + ": " + err.Error()
	}
	u, err := url.Parse(v)
	if err != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", field + " must not carry a query or fragment"
	}
	return v, ""
}

func (rt *Router) auditAsset(r *http.Request, action, bucketID string, detail map[string]any) {
	ptype, pid, _, err := rt.callerPrincipal(r)
	if err != nil {
		rt.logger.Warn("api: asset audit: resolve caller failed", slog.String("bucket_id", bucketID), slog.String("error", err.Error()))
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: asset audit: entry id failed", slog.String("error", err.Error()))
		return
	}
	raw, _ := json.Marshal(detail)
	entry := store.AuditEntry{
		ID: id, ActorType: ptype, ActorID: pid, ActorName: pid, Ability: AbilityWriteSensitive,
		Method: r.Method, Path: r.URL.Path, StatusCode: http.StatusOK, RemoteAddr: clientIP(r),
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action: action, Detail: string(raw),
	}
	if err := rt.auditLog.SaveAuditEntry(r.Context(), entry); err != nil {
		rt.logger.Warn("api: asset audit: save failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}

func assetsUploadMaxBytes() int64 {
	return envPositiveInt64(envAssetsUploadMaxBytes, defaultAssetsUploadMax)
}

// assetsPresignMaxTTL reads a Go duration ("30m") or whole seconds.
func assetsPresignMaxTTL() time.Duration {
	v := strings.TrimSpace(os.Getenv(envAssetsPresignMaxTTL))
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return d
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return defaultAssetsPresignTTL
}
