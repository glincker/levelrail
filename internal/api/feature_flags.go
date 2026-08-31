package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// FeatureFlagStore is the store surface the feature flag handlers need,
// mirroring ScheduledTaskStore's own shape for a different child
// resource.
type FeatureFlagStore interface {
	SaveFeatureFlag(ctx context.Context, f store.FeatureFlag) error
	GetFeatureFlag(ctx context.Context, id string) (store.FeatureFlag, error)
	GetFeatureFlagByKey(ctx context.Context, key string) (store.FeatureFlag, error)
	ListFeatureFlagsForService(ctx context.Context, serviceName string) ([]store.FeatureFlag, error)
	UpdateFeatureFlag(ctx context.Context, id, name, description string, enabled bool, rolloutPercentage int, updatedAt time.Time) error
	DeleteFeatureFlag(ctx context.Context, id string) error
}

// featureFlagResource is the wire shape for a feature flag's full
// metadata (the CRUD routes). ServiceName is always the app name from
// the URL, never taken from the request body, the same "the URL is the
// only thing that gets to say which resource this belongs to" reasoning
// scheduledTaskResource's own doc comment establishes for ServiceName.
type featureFlagResource struct {
	ID                string    `json:"id,omitempty"`
	Key               string    `json:"key"`
	Name              string    `json:"name"`
	Description       string    `json:"description,omitempty"`
	ServiceName       string    `json:"service_name,omitempty"`
	Enabled           bool      `json:"enabled"`
	RolloutPercentage int       `json:"rollout_percentage"`
	CreatedAt         time.Time `json:"created_at,omitempty"`
	UpdatedAt         time.Time `json:"updated_at,omitempty"`
}

func toFeatureFlagResource(f store.FeatureFlag) featureFlagResource {
	return featureFlagResource{
		ID:                f.ID,
		Key:               f.Key,
		Name:              f.Name,
		Description:       f.Description,
		ServiceName:       f.ServiceName,
		Enabled:           f.Enabled,
		RolloutPercentage: f.RolloutPercentage,
		CreatedAt:         f.CreatedAt,
		UpdatedAt:         f.UpdatedAt,
	}
}

// evaluateFlagResource is the tiny wire shape GET
// /api/v1/flags/evaluate/{key} returns: nothing about the flag beyond
// whether this particular evaluation is on, since this is the hot-path
// endpoint a running app calls on every read, potentially often.
type evaluateFlagResource struct {
	Key     string `json:"key"`
	Enabled bool   `json:"enabled"`
}

// validateFeatureFlagKey checks the one field that's immutable after
// create: non-empty, and restricted to the identifier-safe charset a
// caller would actually want to reference from application code (no
// spaces, no characters that would need URL-escaping in the evaluate
// route's path segment).
func validateFeatureFlagKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("key is required")
	}
	for _, r := range key {
		isLower := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		isSep := r == '-' || r == '_'
		if !isLower && !isDigit && !isSep {
			return errors.New("key must contain only lowercase letters, digits, hyphens, and underscores")
		}
	}
	return nil
}

// validateFeatureFlagInput checks the fields a caller controls on both
// create and update: Name and RolloutPercentage. Key is validated
// separately (validateFeatureFlagKey) since update never touches it.
func validateFeatureFlagInput(name string, rolloutPercentage int) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("name is required")
	}
	if rolloutPercentage < 0 || rolloutPercentage > 100 {
		return errors.New("rollout_percentage must be between 0 and 100")
	}
	return nil
}

// handleCreateFeatureFlag handles POST /api/v1/apps/{name}/flags.
func (rt *Router) handleCreateFeatureFlag(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: create feature flag: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req featureFlagResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateFeatureFlagKey(req.Key); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateFeatureFlagInput(req.Name, req.RolloutPercentage); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	key := strings.TrimSpace(req.Key)
	if _, err := rt.featureFlags.GetFeatureFlagByKey(r.Context(), key); err == nil {
		writeError(w, http.StatusConflict, "a feature flag with this key already exists")
		return
	} else if !errors.Is(err, store.ErrFeatureFlagNotFound) {
		rt.logger.Error("api: create feature flag: check existing failed", slog.String("error", err.Error()), slog.String("key", key))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	id, err := randomFeatureFlagID()
	if err != nil {
		rt.logger.Error("api: create feature flag: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	now := time.Now().UTC()
	flag := store.FeatureFlag{
		ID:                id,
		Key:               key,
		Name:              req.Name,
		Description:       req.Description,
		ServiceName:       name,
		Enabled:           req.Enabled,
		RolloutPercentage: req.RolloutPercentage,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := rt.featureFlags.SaveFeatureFlag(r.Context(), flag); err != nil {
		rt.logger.Error("api: create feature flag failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toFeatureFlagResource(flag))
}

// handleListFeatureFlags handles GET /api/v1/apps/{name}/flags.
func (rt *Router) handleListFeatureFlags(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: list feature flags: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	flags, err := rt.featureFlags.ListFeatureFlagsForService(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: list feature flags failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]featureFlagResource, 0, len(flags))
	for _, f := range flags {
		out = append(out, toFeatureFlagResource(f))
	}
	writeJSON(w, http.StatusOK, out)
}

// loadOwnedFeatureFlag loads the feature flag with this id and verifies
// it actually belongs to appName, the same ownership check
// loadOwnedScheduledTask's own doc comment explains for a sibling
// resource: a flag ID doesn't itself encode which app it belongs to, so
// this returns a 404 either way (not found, or belongs to a different
// app) rather than letting a caller with access to app A learn anything
// about an ID belonging to app B.
func (rt *Router) loadOwnedFeatureFlag(w http.ResponseWriter, r *http.Request, appName, id, op string) (store.FeatureFlag, bool) {
	f, err := rt.featureFlags.GetFeatureFlag(r.Context(), id)
	if errors.Is(err, store.ErrFeatureFlagNotFound) {
		writeError(w, http.StatusNotFound, "feature flag not found")
		return store.FeatureFlag{}, false
	}
	if err != nil {
		rt.logger.Error("api: "+op+": load feature flag failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return store.FeatureFlag{}, false
	}
	if f.ServiceName != appName {
		writeError(w, http.StatusNotFound, "feature flag not found")
		return store.FeatureFlag{}, false
	}
	return f, true
}

// handleGetFeatureFlag handles GET /api/v1/apps/{name}/flags/{id}.
func (rt *Router) handleGetFeatureFlag(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	f, ok := rt.loadOwnedFeatureFlag(w, r, name, r.PathValue("id"), "get feature flag")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toFeatureFlagResource(f))
}

// handleUpdateFeatureFlag handles PUT /api/v1/apps/{name}/flags/{id}:
// updates Name/Description/Enabled/RolloutPercentage only, never Key or
// ServiceName (UpdateFeatureFlag's own doc comment).
func (rt *Router) handleUpdateFeatureFlag(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")
	if _, ok := rt.loadOwnedFeatureFlag(w, r, name, id, "update feature flag"); !ok {
		return
	}

	var req featureFlagResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateFeatureFlagInput(req.Name, req.RolloutPercentage); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := rt.featureFlags.UpdateFeatureFlag(r.Context(), id, req.Name, req.Description, req.Enabled, req.RolloutPercentage, time.Now().UTC()); err != nil {
		if errors.Is(err, store.ErrFeatureFlagNotFound) {
			writeError(w, http.StatusNotFound, "feature flag not found")
			return
		}
		rt.logger.Error("api: update feature flag failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	updated, err := rt.featureFlags.GetFeatureFlag(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: update feature flag: reload after update failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toFeatureFlagResource(updated))
}

// handleDeleteFeatureFlag handles DELETE /api/v1/apps/{name}/flags/{id}.
func (rt *Router) handleDeleteFeatureFlag(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")
	if _, ok := rt.loadOwnedFeatureFlag(w, r, name, id, "delete feature flag"); !ok {
		return
	}

	if err := rt.featureFlags.DeleteFeatureFlag(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrFeatureFlagNotFound) {
			writeError(w, http.StatusNotFound, "feature flag not found")
			return
		}
		rt.logger.Error("api: delete feature flag failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleEvaluateFeatureFlag handles GET /api/v1/flags/evaluate/{key}: the
// endpoint a running app's own code calls at runtime to read a flag's
// current value, authenticated the same way as every other AbilityRead
// route (a bearer API token is enough, no new auth surface). Flat, not
// nested under an app, because the token presented has no app scoping
// (internal/store/tokens.go's APIToken), so there is nothing to nest
// under; see migrations/0065_feature_flags.sql for why Key is globally
// unique to make this lookup unambiguous.
func (rt *Router) handleEvaluateFeatureFlag(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")

	f, err := rt.featureFlags.GetFeatureFlagByKey(r.Context(), key)
	if errors.Is(err, store.ErrFeatureFlagNotFound) {
		writeError(w, http.StatusNotFound, "feature flag not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: evaluate feature flag failed", slog.String("error", err.Error()), slog.String("key", key))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	identifier := r.URL.Query().Get("identifier")
	writeJSON(w, http.StatusOK, evaluateFlagResource{
		Key:     f.Key,
		Enabled: evaluateFeatureFlag(f, identifier),
	})
}

// evaluateFeatureFlag reports whether f is on for this evaluation. The
// stored Enabled flag is a hard kill switch: false means off for every
// caller regardless of RolloutPercentage, matching Unleash's own
// "the flag itself gates whether any rollout strategy runs at all"
// convention. When Enabled is true, RolloutPercentage buckets callers
// deterministically via featureFlagBucket rather than a fresh coin flip
// per request, so the same caller sees a stable result across repeated
// calls; 0 and 100 are handled as unconditional cases so they never
// depend on the hash function's own distribution at the edges.
func evaluateFeatureFlag(f store.FeatureFlag, identifier string) bool {
	if !f.Enabled {
		return false
	}
	if f.RolloutPercentage <= 0 {
		return false
	}
	if f.RolloutPercentage >= 100 {
		return true
	}
	return featureFlagBucket(f.ID, identifier) < uint32(f.RolloutPercentage)
}

// featureFlagBucket hashes flagID plus identifier into a stable [0, 100)
// bucket using FNV-1a, the same consistent-hashing approach LaunchDarkly
// and Unleash both use for percentage rollouts: hashing on a bucketing
// key rather than rolling a fresh random number per request is what
// makes repeated evaluations from the same caller land on the same side
// of the rollout. When no caller-supplied identifier is available, this
// falls back to hashing flagID alone: still deterministic (every caller
// without an identifier gets the same outcome), never a random salt that
// would turn RolloutPercentage into a coin flip that changes on every
// call.
func featureFlagBucket(flagID, identifier string) uint32 {
	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "%s:%s", flagID, identifier)
	return h.Sum32() % 100
}

// randomFeatureFlagID mirrors randomScheduledTaskID's exact shape (9
// random bytes, URL-safe base64, a short type prefix). Duplicated rather
// than shared, the same "different resource, different ID space"
// reasoning randomScheduledTaskID's own doc comment gives.
func randomFeatureFlagID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate feature flag id: %w", err)
	}
	return "ff_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
