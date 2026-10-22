package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/functions"
	"github.com/GLINCKER/levelrail/internal/store"
)

const kvPreviewRunes = 120

type functionKVStore interface {
	RotateFunctionKVToken(ctx context.Context, namespace string, now time.Time) (string, error)
	ResolveFunctionKVToken(ctx context.Context, token string) (string, error)
	HasFunctionKVToken(ctx context.Context, namespace string) (bool, error)
	PutFunctionKV(ctx context.Context, namespace, key string, value []byte, ttl time.Duration, lim store.KVLimits, now time.Time) error
	GetFunctionKV(ctx context.Context, namespace, key string, now time.Time) (store.KVEntry, error)
	DeleteFunctionKVKey(ctx context.Context, namespace, key string) (bool, error)
	ListFunctionKV(ctx context.Context, namespace, prefix string, limit int, withValue bool, now time.Time) ([]store.KVEntry, error)
	FunctionKVUsage(ctx context.Context, namespace string, now time.Time) (store.KVUsage, error)
}

func (rt *Router) functionKV() functionKVStore {
	s, _ := rt.appSleep.(functionKVStore)
	return s
}

type kvKeyResource struct {
	Key       string     `json:"key"`
	Size      int        `json:"size"`
	Preview   string     `json:"preview,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type kvLimitsResource struct {
	MaxKeyBytes   int   `json:"max_key_bytes"`
	MaxValueBytes int   `json:"max_value_bytes"`
	MaxKeys       int   `json:"max_keys"`
	MaxTotalBytes int64 `json:"max_total_bytes"`
	MaxTTLSeconds int   `json:"max_ttl_seconds"`
}

type kvListResponse struct {
	Enabled bool             `json:"enabled"`
	Keys    []kvKeyResource  `json:"keys"`
	Count   int              `json:"count"`
	Bytes   int64            `json:"bytes"`
	Limits  kvLimitsResource `json:"limits"`
}

type kvValueResource struct {
	Key       string     `json:"key"`
	Value     string     `json:"value"`
	Size      int        `json:"size"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type kvPutRequest struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	TTLSeconds int    `json:"ttl_seconds"`
}

func kvConfig() functions.KVConfig { return functions.KVConfigFromEnv(nil) }

func kvPreview(b []byte) string {
	if !utf8.Valid(b) {
		return ""
	}
	s := string(b)
	if utf8.RuneCountInString(s) <= kvPreviewRunes {
		return s
	}
	return string([]rune(s)[:kvPreviewRunes]) + "..."
}

func kvKeyFrom(e store.KVEntry, withPreview bool) kvKeyResource {
	out := kvKeyResource{Key: e.Key, Size: e.Size, UpdatedAt: e.UpdatedAt}
	if !e.ExpiresAt.IsZero() {
		exp := e.ExpiresAt
		out.ExpiresAt = &exp
	}
	if withPreview {
		out.Preview = kvPreview(e.Value)
	}
	return out
}

// writeKVError maps store errors to a status; the message never carries a value.
func (rt *Router) writeKVError(w http.ResponseWriter, ns string, err error) {
	switch {
	case errors.Is(err, store.ErrKVNotFound):
		writeError(w, http.StatusNotFound, "key not found")
	case errors.Is(err, store.ErrKVKeyInvalid):
		writeError(w, http.StatusBadRequest, "key is empty or longer than the key size limit")
	case errors.Is(err, store.ErrKVValueTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "value is larger than the value size limit")
	case errors.Is(err, store.ErrKVTooManyKeys):
		writeError(w, http.StatusInsufficientStorage, "the namespace is at its key limit")
	case errors.Is(err, store.ErrKVNamespaceFull):
		writeError(w, http.StatusInsufficientStorage, "the namespace is at its total size limit")
	default:
		rt.internalError(w, "api: function kv failed", err, slog.String("function", ns))
	}
}

func clampTTL(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}
	if ceiling := kvConfig().MaxTTLSeconds; seconds > ceiling {
		seconds = ceiling
	}
	return time.Duration(seconds) * time.Second
}

// handleListFunctionKV handles GET /api/v1/functions/{name}/kv: keys with truncated previews and namespace usage.
func (rt *Router) handleListFunctionKV(w http.ResponseWriter, r *http.Request) {
	svc, _, ok := rt.loadFunction(w, r)
	if !ok {
		return
	}
	kv := rt.functionKV()
	if kv == nil {
		writeError(w, http.StatusNotImplemented, "the key-value store is not available on this control plane")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	now := time.Now()
	entries, err := kv.ListFunctionKV(r.Context(), svc.Name, r.URL.Query().Get("prefix"), limit, true, now)
	if err != nil {
		rt.writeKVError(w, svc.Name, err)
		return
	}
	usage, err := kv.FunctionKVUsage(r.Context(), svc.Name, now)
	if err != nil {
		rt.writeKVError(w, svc.Name, err)
		return
	}
	enabled, err := kv.HasFunctionKVToken(r.Context(), svc.Name)
	if err != nil {
		rt.writeKVError(w, svc.Name, err)
		return
	}
	cfg := kvConfig()
	out := kvListResponse{
		Enabled: enabled, Keys: make([]kvKeyResource, 0, len(entries)), Count: usage.Keys, Bytes: usage.Bytes,
		Limits: kvLimitsResource{MaxKeyBytes: cfg.MaxKeyBytes, MaxValueBytes: cfg.MaxValueBytes, MaxKeys: cfg.MaxKeys, MaxTotalBytes: cfg.MaxTotalBytes, MaxTTLSeconds: cfg.MaxTTLSeconds},
	}
	for _, e := range entries {
		out.Keys = append(out.Keys, kvKeyFrom(e, true))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetFunctionKVValue handles GET /api/v1/functions/{name}/kv/value?key=: one full value.
func (rt *Router) handleGetFunctionKVValue(w http.ResponseWriter, r *http.Request) {
	svc, _, ok := rt.loadFunction(w, r)
	if !ok {
		return
	}
	kv := rt.functionKV()
	if kv == nil {
		writeError(w, http.StatusNotImplemented, "the key-value store is not available on this control plane")
		return
	}
	e, err := kv.GetFunctionKV(r.Context(), svc.Name, r.URL.Query().Get("key"), time.Now())
	if err != nil {
		rt.writeKVError(w, svc.Name, err)
		return
	}
	out := kvValueResource{Key: e.Key, Value: string(e.Value), Size: e.Size}
	if !e.ExpiresAt.IsZero() {
		exp := e.ExpiresAt
		out.ExpiresAt = &exp
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePutFunctionKVValue handles PUT /api/v1/functions/{name}/kv/value: sets a key.
func (rt *Router) handlePutFunctionKVValue(w http.ResponseWriter, r *http.Request) {
	svc, _, ok := rt.loadFunction(w, r)
	if !ok {
		return
	}
	kv := rt.functionKV()
	if kv == nil {
		writeError(w, http.StatusNotImplemented, "the key-value store is not available on this control plane")
		return
	}
	var req kvPutRequest
	if !decodeReqLimit(w, r, &req, int64(kvConfig().MaxValueBytes)*2+4096) {
		return
	}
	if err := kv.PutFunctionKV(r.Context(), svc.Name, req.Key, []byte(req.Value), clampTTL(req.TTLSeconds), kvConfig().KVLimits, time.Now()); err != nil {
		rt.writeKVError(w, svc.Name, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteFunctionKVValue handles DELETE /api/v1/functions/{name}/kv/value?key=.
func (rt *Router) handleDeleteFunctionKVValue(w http.ResponseWriter, r *http.Request) {
	svc, _, ok := rt.loadFunction(w, r)
	if !ok {
		return
	}
	kv := rt.functionKV()
	if kv == nil {
		writeError(w, http.StatusNotImplemented, "the key-value store is not available on this control plane")
		return
	}
	found, err := kv.DeleteFunctionKVKey(r.Context(), svc.Name, r.URL.Query().Get("key"))
	if err != nil {
		rt.writeKVError(w, svc.Name, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRotateFunctionKVToken handles POST /api/v1/functions/{name}/kv/token: enables the store for the
// function or rotates its token. The token is stored as an encrypted secret and never returned.
func (rt *Router) handleRotateFunctionKVToken(w http.ResponseWriter, r *http.Request) {
	svc, _, ok := rt.loadFunction(w, r)
	if !ok {
		return
	}
	kv := rt.functionKV()
	if kv == nil || rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "the key-value store needs the control plane master key")
		return
	}
	token, err := kv.RotateFunctionKVToken(r.Context(), svc.Name, time.Now())
	if err != nil {
		rt.internalError(w, "api: rotate function kv token failed", err, slog.String("function", svc.Name))
		return
	}
	if err := rt.secrets.SetValueGuarded(r.Context(), svc.Name, functions.KVTokenEnv, token, true); err != nil {
		rt.internalError(w, "api: store function kv token failed", err, slog.String("function", svc.Name))
		return
	}
	if _, err := rt.declareSecretEnv(r.Context(), svc.Name, functions.KVTokenEnv, true); err != nil {
		rt.internalError(w, "api: declare function kv token failed", err, slog.String("function", svc.Name))
		return
	}
	env := make(map[string]string, len(svc.Env)+1)
	for k, v := range svc.Env {
		env[k] = v
	}
	env[functions.KVURLEnv] = rt.functionKVBaseURL(r)
	next := *svc
	next.Env = env
	if err := rt.apps.SaveDesiredService(r.Context(), next); err != nil {
		rt.internalError(w, "api: save function kv url failed", err, slog.String("function", svc.Name))
		return
	}
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "rotated_at": time.Now().UTC()})
}

// functionKVBaseURL is the address functions use: APP_FUNCTION_KV_URL when set, else the host this request came in on.
func (rt *Router) functionKVBaseURL(r *http.Request) string {
	if base := strings.TrimRight(os.Getenv(functions.EnvKVURL), "/"); base != "" {
		return base + functions.KVAPIPath
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host + functions.KVAPIPath
}

func (rt *Router) kvNamespaceFromToken(w http.ResponseWriter, r *http.Request) (functionKVStore, string, bool) {
	kv := rt.functionKV()
	if kv == nil {
		writeError(w, http.StatusNotImplemented, "the key-value store is not available on this control plane")
		return nil, "", false
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	ns, err := kv.ResolveFunctionKVToken(r.Context(), strings.TrimSpace(token))
	if errors.Is(err, store.ErrKVTokenInvalid) {
		writeError(w, http.StatusUnauthorized, "missing or invalid key-value token")
		return nil, "", false
	}
	if err != nil {
		rt.internalError(w, "api: resolve function kv token failed", err)
		return nil, "", false
	}
	return kv, ns, true
}

// handleFunctionKVEntry handles GET, PUT and DELETE /api/v1/function-kv/entry?key=. The bearer token fixes the
// namespace, so a function can only ever touch its own keys. PUT takes the raw body as the value and ttl in seconds.
func (rt *Router) handleFunctionKVEntry(w http.ResponseWriter, r *http.Request) {
	kv, ns, ok := rt.kvNamespaceFromToken(w, r)
	if !ok {
		return
	}
	key := r.URL.Query().Get("key")
	cfg := kvConfig()
	now := time.Now()
	switch r.Method {
	case http.MethodGet:
		e, err := kv.GetFunctionKV(r.Context(), ns, key, now)
		if err != nil {
			rt.writeKVError(w, ns, err)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		if !e.ExpiresAt.IsZero() {
			w.Header().Set("X-KV-Expires-At", e.ExpiresAt.Format(time.RFC3339))
		}
		_, _ = w.Write(e.Value)
	case http.MethodPut:
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(cfg.MaxValueBytes)+1))
		if err != nil {
			writeError(w, http.StatusRequestEntityTooLarge, "value is larger than the value size limit")
			return
		}
		ttl, _ := strconv.Atoi(r.URL.Query().Get("ttl"))
		if err := kv.PutFunctionKV(r.Context(), ns, key, body, clampTTL(ttl), cfg.KVLimits, now); err != nil {
			rt.writeKVError(w, ns, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if _, err := kv.DeleteFunctionKVKey(r.Context(), ns, key); err != nil {
			rt.writeKVError(w, ns, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleFunctionKVKeys handles GET /api/v1/function-kv/keys?prefix=&limit=: key names only, for the token's namespace.
func (rt *Router) handleFunctionKVKeys(w http.ResponseWriter, r *http.Request) {
	kv, ns, ok := rt.kvNamespaceFromToken(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	entries, err := kv.ListFunctionKV(r.Context(), ns, r.URL.Query().Get("prefix"), limit, false, time.Now())
	if err != nil {
		rt.writeKVError(w, ns, err)
		return
	}
	keys := make([]kvKeyResource, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, kvKeyFrom(e, false))
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}
