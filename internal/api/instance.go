package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/version"
)

const (
	envInstanceInfoRate       = "APP_INSTANCE_INFO_RATE_PER_MINUTE" //nolint:gosec // env var name
	defaultInstanceInfoRate   = 60
	instanceCacheTTL          = 5 * time.Second
	confirmSameInstancePhrase = "same instance"
)

// InstanceStore is the identity and token-binding surface of the store.
type InstanceStore interface {
	GetInstanceInfo(ctx context.Context) (store.InstanceInfo, error)
	ConfirmSameInstance(ctx context.Context) error
	BindOrCheckToken(ctx context.Context, tokenID, instanceID, ip string) (bool, error)
	ListTokenBindings(ctx context.Context) (map[string]store.TokenBinding, error)
}

func instanceInfoRatePerMinute() int {
	if n, err := strconv.Atoi(os.Getenv(envInstanceInfoRate)); err == nil {
		return n
	}
	return defaultInstanceInfoRate
}

// instanceCache keeps the identity for a few seconds so the token middleware does not
// read it on every request; confirm-same invalidates it.
type instanceCache struct {
	mu   sync.Mutex
	info store.InstanceInfo
	at   time.Time
}

func (rt *Router) instanceInfo(ctx context.Context) (store.InstanceInfo, error) {
	c := rt.instanceMemo
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.info.InstanceID != "" && time.Since(c.at) < instanceCacheTTL {
		return c.info, nil
	}
	info, err := rt.instance.GetInstanceInfo(ctx)
	if err != nil {
		return store.InstanceInfo{}, err
	}
	c.info, c.at = info, time.Now()
	return info, nil
}

func (rt *Router) invalidateInstanceInfo() {
	rt.instanceMemo.mu.Lock()
	rt.instanceMemo.at = time.Time{}
	rt.instanceMemo.mu.Unlock()
}

type instanceResource struct {
	InstanceID  string `json:"instance_id"`
	Name        string `json:"name"`
	ShortName   string `json:"short_name"`
	Version     string `json:"version"`
	Fingerprint string `json:"fingerprint"`
}

type instanceIdentityResource struct {
	instanceResource
	Moved              bool   `json:"moved"`
	MovedAt            string `json:"moved_at,omitempty"`
	PreviousInstanceID string `json:"previous_instance_id,omitempty"`
}

func (rt *Router) instanceResourceFrom(info store.InstanceInfo) instanceResource {
	return instanceResource{
		InstanceID: info.InstanceID, Name: rt.brand.Name, ShortName: rt.brand.ShortName,
		Version: version.Version, Fingerprint: info.Fingerprint,
	}
}

// handleInstance handles GET /api/v1/instance. Public so a client can verify which server
// it reached before sending a token; it returns no secret, only the HMAC fingerprint.
func (rt *Router) handleInstance(w http.ResponseWriter, r *http.Request) {
	if rt.instanceLimit != nil {
		if ok, retryAfter := rt.instanceLimit.allow(clientIP(r)); !ok {
			writeRateLimited(w, retryAfter)
			return
		}
	}
	info, err := rt.instanceInfo(r.Context())
	if err != nil {
		rt.internalError(w, "api: instance info failed", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, rt.instanceResourceFrom(info))
}

// handleInstanceIdentity handles GET /api/v1/instance/identity: the signed-in view,
// including whether this instance was moved and is waiting for confirmation.
func (rt *Router) handleInstanceIdentity(w http.ResponseWriter, r *http.Request) {
	info, err := rt.instanceInfo(r.Context())
	if err != nil {
		rt.internalError(w, "api: instance identity failed", err)
		return
	}
	writeJSON(w, http.StatusOK, instanceIdentityResource{
		instanceResource: rt.instanceResourceFrom(info),
		Moved:            info.PreviousInstanceID != "", MovedAt: info.MovedAt, PreviousInstanceID: info.PreviousInstanceID,
	})
}

type confirmSameInstanceRequest struct {
	Confirm string `json:"confirm"`
}

// handleConfirmSameInstance handles POST /api/v1/instance/confirm-same: after a restore
// onto a new host the identity rotated and every bound CLI token stopped working; a root
// operator who typed the confirmation phrase restores the old identity.
func (rt *Router) handleConfirmSameInstance(w http.ResponseWriter, r *http.Request) {
	var req confirmSameInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Confirm != confirmSameInstancePhrase {
		writeError(w, http.StatusBadRequest, `type "`+confirmSameInstancePhrase+`" to confirm this is the same instance`)
		return
	}
	err := rt.instance.ConfirmSameInstance(r.Context())
	if errors.Is(err, store.ErrNoPreviousInstance) {
		writeError(w, http.StatusConflict, "this instance has not moved, nothing to confirm")
		return
	}
	if err != nil {
		rt.internalError(w, "api: confirm same instance failed", err)
		return
	}
	rt.invalidateInstanceInfo()
	rt.logger.Warn("api: instance identity restored by operator confirmation")
	w.WriteHeader(http.StatusNoContent)
}

// tokenBindingRejects binds an unbound token to this instance on first use and refuses
// one bound to another instance (a cloned or restored database). It writes the response.
func (rt *Router) tokenBindingRejects(w http.ResponseWriter, r *http.Request, rec *store.APIToken) bool {
	info, err := rt.instanceInfo(r.Context())
	if err != nil {
		rt.internalError(w, "api: instance identity lookup failed", err)
		return true
	}
	ok, err := rt.instance.BindOrCheckToken(r.Context(), rec.ID, info.InstanceID, clientIP(r))
	if err != nil && !ok {
		rt.internalError(w, "api: token binding check failed", err, slog.String("token_id", rec.ID))
		return true
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, "token is bound to another server instance; run `"+rt.brand.BinaryName+" auth login --reauth` against this server")
		return true
	}
	return false
}
