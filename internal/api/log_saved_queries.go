package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	envSavedLogQueryMax     = "APP_LOG_SAVED_QUERY_MAX"
	defaultSavedLogQueryMax = 100
	maxLogQueryNameLen      = 80
	maxLogQueryTextLen      = 500
	maxLogQueryRange        = 30 * 24 * time.Hour
)

var logQueryRangeRE = regexp.MustCompile(`^(\d{1,4})([mhd])$`)

type savedLogQueryStore interface {
	CreateSavedLogQuery(ctx context.Context, q store.SavedLogQuery) error
	UpdateSavedLogQuery(ctx context.Context, q store.SavedLogQuery) error
	GetSavedLogQuery(ctx context.Context, id string) (store.SavedLogQuery, error)
	ListSavedLogQueries(ctx context.Context, ownerID, app string) ([]store.SavedLogQuery, error)
	DeleteSavedLogQuery(ctx context.Context, id string) error
}

func (rt *Router) savedLogQueryStore() savedLogQueryStore {
	s, _ := rt.apps.(savedLogQueryStore)
	return s
}

// logQuerySpec is the search a saved query (or a permalink) reproduces.
// Range is relative ("15m", "1h", "7d"); From and To pin an absolute window.
type logQuerySpec struct {
	Q         string     `json:"q,omitempty"`
	Level     string     `json:"level,omitempty"`
	Stream    string     `json:"stream,omitempty"`
	Fields    []string   `json:"fields,omitempty"`
	Container string     `json:"container,omitempty"`
	Range     string     `json:"range,omitempty"`
	From      *time.Time `json:"from,omitempty"`
	To        *time.Time `json:"to,omitempty"`
}

func (s *logQuerySpec) validate() error {
	if len(s.Q) > maxLogQueryTextLen {
		return fmt.Errorf("q must be at most %d characters", maxLogQueryTextLen)
	}
	if s.Level != "" {
		l, err := parseLevelParam(s.Level)
		if err != nil {
			return err
		}
		s.Level = l
	}
	if s.Stream != "" && s.Stream != "stdout" && s.Stream != "stderr" {
		return errors.New("stream must be stdout or stderr")
	}
	if _, err := telemetry.ParseFieldFilters(s.Fields); err != nil {
		return err
	}
	if s.Range != "" {
		if _, err := parseLogQueryRange(s.Range); err != nil {
			return err
		}
	}
	if s.From != nil && s.To != nil && s.From.After(*s.To) {
		return errors.New("from must not be after to")
	}
	return nil
}

func parseLogQueryRange(raw string) (time.Duration, error) {
	m := logQueryRangeRE.FindStringSubmatch(raw)
	if m == nil {
		return 0, errors.New("range must look like 15m, 1h or 7d")
	}
	n, _ := strconv.Atoi(m[1])
	unit := map[string]time.Duration{"m": time.Minute, "h": time.Hour, "d": 24 * time.Hour}[m[2]]
	d := time.Duration(n) * unit
	if d <= 0 || d > maxLogQueryRange {
		return 0, fmt.Errorf("range must be between 1m and %dd", int(maxLogQueryRange/(24*time.Hour)))
	}
	return d, nil
}

type savedLogQueryRequest struct {
	Name       string       `json:"name"`
	App        string       `json:"app"`
	Spec       logQuerySpec `json:"spec"`
	Visibility string       `json:"visibility"`
	Pinned     bool         `json:"pinned"`
}

type savedLogQueryResource struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	App        string       `json:"app,omitempty"`
	Spec       logQuerySpec `json:"spec"`
	Visibility string       `json:"visibility"`
	Pinned     bool         `json:"pinned"`
	Owner      string       `json:"owner,omitempty"`
	Mine       bool         `json:"mine"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
}

func toSavedLogQueryResource(q store.SavedLogQuery, callerID string) savedLogQueryResource {
	var spec logQuerySpec
	_ = json.Unmarshal([]byte(q.SpecJSON), &spec)
	return savedLogQueryResource{
		ID: q.ID, Name: q.Name, App: q.App, Spec: spec, Visibility: q.Visibility, Pinned: q.Pinned,
		Owner: q.OwnerName, Mine: q.OwnerID == callerID, CreatedAt: q.CreatedAt, UpdatedAt: q.UpdatedAt,
	}
}

func savedLogQueryMax() int {
	if n, err := strconv.Atoi(os.Getenv(envSavedLogQueryMax)); err == nil && n > 0 {
		return n
	}
	return defaultSavedLogQueryMax
}

// savedQueryCaller resolves the caller or writes the failure response.
func (rt *Router) savedQueryCaller(w http.ResponseWriter, r *http.Request) (principalType, id string, ok bool) {
	pt, pid, _, err := rt.callerPrincipal(r)
	if err != nil || pid == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return "", "", false
	}
	return pt, pid, true
}

// canSeeSavedQuery applies ownership, sharing and app visibility.
func (rt *Router) canSeeSavedQuery(q store.SavedLogQuery, callerID string, canSeeApp func(string) bool) bool {
	if q.OwnerID != callerID && q.Visibility != store.SavedLogQueryInstance {
		return false
	}
	return q.App == "" || canSeeApp(q.App)
}

func (rt *Router) handleListSavedLogQueries(w http.ResponseWriter, r *http.Request) {
	st := rt.savedLogQueryStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "saved log queries are not supported by this store")
		return
	}
	_, callerID, ok := rt.savedQueryCaller(w, r)
	if !ok {
		return
	}
	canSeeApp, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: list saved log queries: visibility", err)
		return
	}
	app := r.URL.Query().Get("app")
	if app != "" && !canSeeApp(app) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	rows, err := st.ListSavedLogQueries(r.Context(), callerID, app)
	if err != nil {
		rt.internalError(w, "api: list saved log queries", err)
		return
	}
	out := make([]savedLogQueryResource, 0, len(rows))
	for _, q := range rows {
		if rt.canSeeSavedQuery(q, callerID, canSeeApp) {
			out = append(out, toSavedLogQueryResource(q, callerID))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"queries": out})
}

// validateSavedQueryRequest checks the body and the caller's right to scope
// and share it, writing the failure response itself.
func (rt *Router) validateSavedQueryRequest(w http.ResponseWriter, r *http.Request, req *savedLogQueryRequest) bool {
	req.Name = strings.TrimSpace(req.Name)
	switch {
	case req.Name == "" || len(req.Name) > maxLogQueryNameLen:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("name is required and at most %d characters", maxLogQueryNameLen))
		return false
	case req.Visibility == "":
		req.Visibility = store.SavedLogQueryPrivate
	case req.Visibility != store.SavedLogQueryPrivate && req.Visibility != store.SavedLogQueryInstance:
		writeError(w, http.StatusBadRequest, "visibility must be private or instance")
		return false
	}
	if req.Pinned && req.App == "" {
		writeError(w, http.StatusBadRequest, "a pinned query needs an app")
		return false
	}
	if err := req.Spec.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	if req.Visibility == store.SavedLogQueryInstance && !rt.callerHasAbility(r, AbilityWrite) {
		writeError(w, http.StatusForbidden, "sharing a query needs write access")
		return false
	}
	if req.App != "" {
		canSeeApp, err := rt.appVisibilityFilter(r)
		if err != nil {
			rt.internalError(w, "api: saved log query: visibility", err)
			return false
		}
		if !canSeeApp(req.App) {
			writeError(w, http.StatusNotFound, "app not found")
			return false
		}
		if _, found, err := rt.lookupAppResource(r.Context(), req.App); err != nil {
			rt.internalError(w, "api: saved log query: look up app", err)
			return false
		} else if !found {
			writeError(w, http.StatusNotFound, "app not found")
			return false
		}
	}
	return true
}

func (rt *Router) handleCreateSavedLogQuery(w http.ResponseWriter, r *http.Request) {
	st := rt.savedLogQueryStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "saved log queries are not supported by this store")
		return
	}
	pt, callerID, ok := rt.savedQueryCaller(w, r)
	if !ok {
		return
	}
	var req savedLogQueryRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !rt.validateSavedQueryRequest(w, r, &req) {
		return
	}
	owned, err := st.ListSavedLogQueries(r.Context(), callerID, "")
	if err != nil {
		rt.internalError(w, "api: create saved log query: count", err)
		return
	}
	count := 0
	for _, q := range owned {
		if q.OwnerID == callerID {
			count++
		}
	}
	if count >= savedLogQueryMax() {
		writeError(w, http.StatusConflict, fmt.Sprintf("at most %d saved queries per user", savedLogQueryMax()))
		return
	}
	id, err := store.NewSavedLogQueryID()
	if err != nil {
		rt.internalError(w, "api: create saved log query: id", err)
		return
	}
	spec, _ := json.Marshal(req.Spec)
	q := store.SavedLogQuery{
		ID: id, Name: req.Name, App: req.App, SpecJSON: string(spec), OwnerType: pt, OwnerID: callerID,
		OwnerName: rt.callerDisplayName(r), Visibility: req.Visibility, Pinned: req.Pinned,
	}
	if err := st.CreateSavedLogQuery(r.Context(), q); errors.Is(err, store.ErrSavedLogQueryExists) {
		writeError(w, http.StatusConflict, "you already have a query with that name")
		return
	} else if err != nil {
		rt.internalError(w, "api: create saved log query", err)
		return
	}
	saved, err := st.GetSavedLogQuery(r.Context(), id)
	if err != nil {
		saved = q
	}
	writeJSON(w, http.StatusCreated, toSavedLogQueryResource(saved, callerID))
}

// loadOwnSavedQuery loads {id} for a mutation: only the owner or a root
// caller may change a query, and a private one of someone else is a 404.
func (rt *Router) loadOwnSavedQuery(w http.ResponseWriter, r *http.Request) (savedLogQueryStore, store.SavedLogQuery, string, bool) {
	st := rt.savedLogQueryStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "saved log queries are not supported by this store")
		return nil, store.SavedLogQuery{}, "", false
	}
	_, callerID, ok := rt.savedQueryCaller(w, r)
	if !ok {
		return nil, store.SavedLogQuery{}, "", false
	}
	q, err := st.GetSavedLogQuery(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrSavedLogQueryNotFound) {
		writeError(w, http.StatusNotFound, "saved query not found")
		return nil, q, "", false
	} else if err != nil {
		rt.internalError(w, "api: load saved log query", err)
		return nil, q, "", false
	}
	if q.OwnerID != callerID && q.Visibility != store.SavedLogQueryInstance {
		writeError(w, http.StatusNotFound, "saved query not found")
		return nil, q, "", false
	}
	if q.OwnerID != callerID && !rt.callerHasAbility(r, AbilityRoot) {
		writeError(w, http.StatusForbidden, "only the owner can change a shared query")
		return nil, q, "", false
	}
	return st, q, callerID, true
}

func (rt *Router) handleUpdateSavedLogQuery(w http.ResponseWriter, r *http.Request) {
	st, q, callerID, ok := rt.loadOwnSavedQuery(w, r)
	if !ok {
		return
	}
	var req savedLogQueryRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !rt.validateSavedQueryRequest(w, r, &req) {
		return
	}
	spec, _ := json.Marshal(req.Spec)
	q.Name, q.App, q.SpecJSON, q.Visibility, q.Pinned = req.Name, req.App, string(spec), req.Visibility, req.Pinned
	if err := st.UpdateSavedLogQuery(r.Context(), q); errors.Is(err, store.ErrSavedLogQueryExists) {
		writeError(w, http.StatusConflict, "a query with that name already exists")
		return
	} else if err != nil {
		rt.internalError(w, "api: update saved log query", err)
		return
	}
	writeJSON(w, http.StatusOK, toSavedLogQueryResource(q, callerID))
}

func (rt *Router) handleDeleteSavedLogQuery(w http.ResponseWriter, r *http.Request) {
	st, q, _, ok := rt.loadOwnSavedQuery(w, r)
	if !ok {
		return
	}
	if err := st.DeleteSavedLogQuery(r.Context(), q.ID); err != nil && !errors.Is(err, store.ErrSavedLogQueryNotFound) {
		rt.internalError(w, "api: delete saved log query", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
