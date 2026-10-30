package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/apptype"
	"github.com/GLINCKER/levelrail/internal/policy"
	"github.com/GLINCKER/levelrail/internal/store"
)

// appTypeStore is satisfied by *store.DB.
type appTypeStore interface {
	SaveAppTypeDraft(ctx context.Context, name, document, by string) error
	GetAppTypeDraft(ctx context.Context, name string) (store.AppTypeDraft, error)
	ListAppTypes(ctx context.Context) ([]store.AppTypeSummary, error)
	GetAppTypeVersion(ctx context.Context, name string, version int) (store.AppTypeVersion, error)
	CountAppTypeVersions(ctx context.Context, name string) (int, error)
	InsertAppTypeVersion(ctx context.Context, v store.AppTypeVersion) error
}

func (rt *Router) appTypeDB() appTypeStore {
	for _, candidate := range []any{rt.apps, rt.projects, rt.policies} {
		if s, ok := candidate.(appTypeStore); ok {
			return s
		}
	}
	return nil
}

// appTypeGate answers 404 while the feature flag is off, like other gated
// features, and 501 when the store cannot hold types.
func (rt *Router) appTypeGate(w http.ResponseWriter) (appTypeStore, bool) {
	if !apptype.Enabled() {
		writeError(w, http.StatusNotFound, "not found")
		return nil, false
	}
	db := rt.appTypeDB()
	if db == nil {
		writeError(w, http.StatusNotImplemented, "app types are not available on this control plane")
	}
	return db, db != nil
}

type appTypeResource struct {
	Name          string          `json:"name"`
	LatestVersion int             `json:"latest_version"`
	UpdatedBy     string          `json:"updated_by,omitempty"`
	UpdatedAt     *time.Time      `json:"updated_at,omitempty"`
	Draft         json.RawMessage `json:"draft,omitempty"`
}

type appTypePublished struct {
	Name        string     `json:"name"`
	Version     int        `json:"version"`
	Digest      string     `json:"digest"`
	PublishedBy string     `json:"published_by,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
}

// handleListAppTypes handles GET /api/v1/app-types.
func (rt *Router) handleListAppTypes(w http.ResponseWriter, r *http.Request) {
	db, ok := rt.appTypeGate(w)
	if !ok {
		return
	}
	list, err := db.ListAppTypes(r.Context())
	if err != nil {
		rt.internalError(w, "api: app types: list", err)
		return
	}
	out := make([]appTypeResource, 0, len(list))
	for _, s := range list {
		at := s.UpdatedAt
		out = append(out, appTypeResource{Name: s.Name, LatestVersion: s.LatestVersion, UpdatedBy: s.UpdatedBy, UpdatedAt: &at})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSaveAppType handles POST /api/v1/app-types: it validates the
// document and stores it as the draft. Nothing is enforced until publish.
func (rt *Router) handleSaveAppType(w http.ResponseWriter, r *http.Request) {
	db, ok := rt.appTypeGate(w)
	if !ok {
		return
	}
	var raw json.RawMessage
	if err := decodeJSONBody(r, &raw); err != nil || len(raw) == 0 {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	t, err := apptype.ParseDocument(raw, apptype.LimitsFromEnv())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := db.SaveAppTypeDraft(r.Context(), t.Name, string(raw), rt.eventActor(r)); err != nil {
		rt.internalError(w, "api: app types: save", err)
		return
	}
	writeJSON(w, http.StatusOK, appTypeResource{Name: t.Name, Draft: raw})
}

type appTypePublishBody struct {
	// Version asks for a specific number; an existing one is refused.
	Version int `json:"version,omitempty"`
}

// handlePublishAppType handles POST /api/v1/app-types/{name}/publish. The
// draft is frozen as the next integer version; the audit middleware records
// the call because publishing needs the root ability.
func (rt *Router) handlePublishAppType(w http.ResponseWriter, r *http.Request) {
	db, ok := rt.appTypeGate(w)
	if !ok {
		return
	}
	name := r.PathValue("name")
	var body appTypePublishBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	draft, err := db.GetAppTypeDraft(r.Context(), name)
	if errors.Is(err, store.ErrAppTypeNotFound) {
		writeError(w, http.StatusNotFound, "app type not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: app types: draft", err)
		return
	}
	lim := apptype.LimitsFromEnv()
	t, err := apptype.ParseDocument([]byte(draft.Document), lim)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	count, err := db.CountAppTypeVersions(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: app types: count", err)
		return
	}
	if count >= lim.MaxVersions {
		writeError(w, http.StatusConflict, "this type already has the maximum number of published versions")
		return
	}
	version := body.Version
	if version == 0 {
		version = nextAppTypeVersion(r.Context(), db, name)
	}
	t.Version = version
	doc, err := json.Marshal(t)
	if err != nil {
		rt.internalError(w, "api: app types: encode", err)
		return
	}
	sum := sha256.Sum256(doc)
	pub := store.AppTypeVersion{Name: name, Version: version, Document: string(doc), Digest: hex.EncodeToString(sum[:]), PublishedBy: rt.eventActor(r)}
	if err := db.InsertAppTypeVersion(r.Context(), pub); errors.Is(err, store.ErrAppTypeVersionExists) {
		writeError(w, http.StatusConflict, "version "+strconv.Itoa(version)+" is already published and cannot be changed")
		return
	} else if err != nil {
		rt.internalError(w, "api: app types: publish", err)
		return
	}
	writeJSON(w, http.StatusCreated, appTypePublished{Name: name, Version: version, Digest: pub.Digest, PublishedBy: pub.PublishedBy})
}

func nextAppTypeVersion(ctx context.Context, db appTypeStore, name string) int {
	latest, err := db.GetAppTypeVersion(ctx, name, 0)
	if err != nil {
		return 1
	}
	return latest.Version + 1
}

type appTypeCheckBody struct {
	// Version defaults to the newest published one.
	Version int          `json:"version,omitempty"`
	Spec    apptype.Spec `json:"spec"`
}

type appTypeCheckResult struct {
	Name       string             `json:"name"`
	Version    int                `json:"version"`
	Violations []policy.Violation `json:"violations"`
	WouldFail  bool               `json:"would_fail"`
}

// handleCheckAppType handles POST /api/v1/app-types/{name}/check: a dry run
// of one app spec against a published version. It never changes anything.
func (rt *Router) handleCheckAppType(w http.ResponseWriter, r *http.Request) {
	db, ok := rt.appTypeGate(w)
	if !ok {
		return
	}
	var body appTypeCheckBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	pub, err := db.GetAppTypeVersion(r.Context(), r.PathValue("name"), body.Version)
	if errors.Is(err, store.ErrAppTypeNotFound) {
		writeError(w, http.StatusNotFound, "no published version of this app type")
		return
	}
	if err != nil {
		rt.internalError(w, "api: app types: version", err)
		return
	}
	var t apptype.AppType
	if err := json.Unmarshal([]byte(pub.Document), &t); err != nil {
		rt.internalError(w, "api: app types: decode", err)
		return
	}
	violations := apptype.Check(t, body.Spec)
	if violations == nil {
		violations = []policy.Violation{}
	}
	res := appTypeCheckResult{Name: pub.Name, Version: pub.Version, Violations: violations}
	for _, v := range violations {
		res.WouldFail = res.WouldFail || v.Blocks()
	}
	writeJSON(w, http.StatusOK, res)
}
