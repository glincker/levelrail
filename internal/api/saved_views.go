package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Audit actions for saved list views; rows carry the view name and list, never the filter.
const (
	AuditActionSavedViewCreate = "saved_view.create"
	AuditActionSavedViewUpdate = "saved_view.update"
	AuditActionSavedViewShare  = "saved_view.share"
	AuditActionSavedViewDelete = "saved_view.delete"
)

// savedViewKeys is the per list allowlist of filter keys; a view can only ever carry these URL params.
var savedViewKeys = map[string][]string{
	"apps":        {"q", "status", "env", "tags", "labels"},
	"databases":   {"q", "status", "engine", "tag", "labels"},
	"domains":     {"q", "status"},
	"builds":      {"q", "status", "app"},
	"deployments": {"q", "status", "app", "env"},
}

const savedViewBodyLimit = 64 << 10

const msgSavedViewNotFound = "saved view not found"

// savedViewStore keeps saved list views; *store.DB satisfies it.
type savedViewStore interface {
	UpsertSavedView(ctx context.Context, v store.SavedView, maxPerOwner int) (store.SavedView, error)
	FindSavedView(ctx context.Context, owner, list, name string) (store.SavedView, error)
	GetSavedView(ctx context.Context, id string) (store.SavedView, error)
	ListSavedViews(ctx context.Context, f store.SavedViewFilter) ([]store.SavedView, error)
	DeleteSavedView(ctx context.Context, id string) error
}

type savedViewRequest struct {
	Name   string            `json:"name"`
	List   string            `json:"list"`
	Filter map[string]string `json:"filter"`
	Shared string            `json:"shared"`
}

type savedViewResource struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	List      string            `json:"list"`
	Filter    map[string]string `json:"filter"`
	Shared    string            `json:"shared"`
	Mine      bool              `json:"mine"`
	CanDelete bool              `json:"can_delete"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
}

func (rt *Router) savedViews() savedViewStore {
	s, _ := rt.apps.(savedViewStore)
	return s
}

// savedViewCaller is who acts on saved views and the team scope the gate resolved for them.
type savedViewCaller struct {
	owner string
	scope *teamScope
}

// resolveSavedViewCaller keys views by user, so a user's tokens share that user's views.
func (rt *Router) resolveSavedViewCaller(r *http.Request) (savedViewCaller, error) {
	scope, ok := callerTeamScope(r.Context())
	// A session link resolves as a token here, so it maps through the same token owner lookup.
	pt, pid, _, err := rt.callerPrincipal(r)
	if err != nil {
		return savedViewCaller{}, err
	}
	if !ok {
		if scope, err = rt.principalTeamScope(r.Context(), pt, pid); err != nil {
			return savedViewCaller{}, err
		}
	}
	owner := "user:" + pid
	if pt == store.PrincipalTypeToken {
		owner = "token:" + pid
		if rt.tokens != nil {
			if rec, terr := rt.tokens.GetAPITokenByID(r.Context(), pid); terr == nil && rec != nil && rec.OwnerUserID != "" {
				owner = "user:" + rec.OwnerUserID
			}
		}
	}
	return savedViewCaller{owner: owner, scope: scope}, nil
}

// homeTeam is the team a new view belongs to, taken from the session and switcher, never from the body.
func (c savedViewCaller) homeTeam(r *http.Request) string {
	requested := requestScopeValue(r, scopeTeamHeader, scopeTeamCookie)
	if c.scope == nil {
		if requested != "" {
			return requested
		}
		return store.DefaultTeamID
	}
	if requested != "" && c.scope.roleOfTeam(requested) != "" {
		return requested
	}
	var teams []string
	for id, role := range c.scope.roles {
		if role != "" {
			teams = append(teams, id)
		}
	}
	if slices.Contains(teams, store.DefaultTeamID) {
		return store.DefaultTeamID
	}
	sort.Strings(teams)
	if len(teams) == 0 {
		return ""
	}
	return teams[0]
}

func (c savedViewCaller) roleIn(teamID string) string {
	if c.scope == nil {
		return store.TeamRoleOwner
	}
	return c.scope.roleOfTeam(teamID)
}

// teamVisible keeps a team limited token away from its owner's views in other teams.
func (c savedViewCaller) teamVisible(teamID string) bool {
	return teamID == "" || c.roleIn(teamID) != ""
}

// canRead is the only visibility rule: the owner within its scope, or a member of the team a view is shared with.
func (c savedViewCaller) canRead(v store.SavedView) bool {
	if v.Owner == c.owner {
		return c.teamVisible(v.TeamID)
	}
	return v.Shared == store.SavedViewTeam && v.TeamID != "" && c.roleIn(v.TeamID) != ""
}

func (c savedViewCaller) canDelete(v store.SavedView) bool {
	if v.Owner == c.owner {
		return c.teamVisible(v.TeamID)
	}
	return v.Shared == store.SavedViewTeam && v.TeamID != "" && store.TeamRoleAtLeast(c.roleIn(v.TeamID), store.TeamRoleAdmin)
}

func (c savedViewCaller) resource(v store.SavedView) savedViewResource {
	filter := map[string]string{}
	_ = json.Unmarshal([]byte(v.Filter), &filter)
	return savedViewResource{
		ID: v.ID, Name: v.Name, List: v.List, Filter: filter, Shared: v.Shared,
		Mine: v.Owner == c.owner, CanDelete: c.canDelete(v), CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

type savedViewCaps struct {
	nameChars, maxKeys, valueBytes, perUser int
}

func (rt *Router) savedViewCaps(ctx context.Context) savedViewCaps {
	s := rt.runtimeSettings
	return savedViewCaps{
		nameChars:  s.Limit(ctx, runtimesettings.KeySavedViewNameMaxChars),
		maxKeys:    s.Limit(ctx, runtimesettings.KeySavedViewFilterMaxKeys),
		valueBytes: s.Limit(ctx, runtimesettings.KeySavedViewValueMaxBytes),
		perUser:    s.Limit(ctx, runtimesettings.KeySavedViewMaxPerUser),
	}
}

// plainTextOK refuses control, format (bidi) and markup characters, so a name or value is inert wherever it shows.
func plainTextOK(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '<' || r == '>' {
			return false
		}
	}
	return true
}

func validateSavedView(req savedViewRequest, caps savedViewCaps) (savedViewRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return req, errors.New("name is required")
	}
	if n := utf8.RuneCountInString(req.Name); n > caps.nameChars {
		return req, fmt.Errorf("name is %d characters, at most %d are allowed", n, caps.nameChars)
	}
	if !plainTextOK(req.Name) {
		return req, errors.New("name must be plain text without control, format or markup characters")
	}
	allowed, ok := savedViewKeys[req.List]
	if !ok {
		return req, errors.New("list must be one of apps, databases, domains, builds, deployments")
	}
	switch req.Shared {
	case "":
		req.Shared = store.SavedViewPrivate
	case store.SavedViewPrivate, store.SavedViewTeam:
	default:
		return req, errors.New("shared must be private or team")
	}
	if len(req.Filter) > caps.maxKeys {
		return req, fmt.Errorf("filter has %d keys, at most %d are allowed", len(req.Filter), caps.maxKeys)
	}
	clean := make(map[string]string, len(req.Filter))
	for k, v := range req.Filter {
		if !slices.Contains(allowed, k) {
			return req, fmt.Errorf("filter key is not allowed for %s; allowed: %s", req.List, strings.Join(allowed, ", "))
		}
		if len(v) > caps.valueBytes {
			return req, fmt.Errorf("filter value of %s is %d bytes, at most %d are allowed", k, len(v), caps.valueBytes)
		}
		if !plainTextOK(v) {
			return req, fmt.Errorf("filter value of %s must be plain text without control, format or markup characters", k)
		}
		if v = strings.TrimSpace(v); v != "" {
			clean[k] = v
		}
	}
	req.Filter = clean
	return req, nil
}

// handleListSavedViews handles GET /api/v1/saved-views: the caller's own views and the views shared with its teams.
func (rt *Router) handleListSavedViews(w http.ResponseWriter, r *http.Request) {
	vs := rt.savedViews()
	if vs == nil {
		writeJSON(w, http.StatusOK, map[string]any{"views": []savedViewResource{}})
		return
	}
	list := r.URL.Query().Get("list")
	if _, ok := savedViewKeys[list]; list != "" && !ok {
		writeError(w, http.StatusBadRequest, "list must be one of apps, databases, domains, builds, deployments")
		return
	}
	c, err := rt.resolveSavedViewCaller(r)
	if err != nil {
		rt.internalError(w, "api: saved views: resolve caller", err)
		return
	}
	narrowed := narrowTeamScope(r, c.scope)
	f := store.SavedViewFilter{Owner: c.owner, List: list, AllTeams: narrowed == nil || narrowed.allTeams}
	if !f.AllTeams {
		for id, role := range narrowed.roles {
			if role != "" {
				f.Teams = append(f.Teams, id)
			}
		}
		sort.Strings(f.Teams)
	}
	views, err := vs.ListSavedViews(r.Context(), f)
	if err != nil {
		rt.internalError(w, "api: list saved views", err)
		return
	}
	out := make([]savedViewResource, 0, len(views))
	for _, v := range views {
		if c.canRead(v) {
			out = append(out, c.resource(v))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"views": out})
}

// handleSaveView handles POST /api/v1/saved-views: create or replace the caller's view of that list and name.
func (rt *Router) handleSaveView(w http.ResponseWriter, r *http.Request) {
	vs := rt.savedViews()
	if vs == nil {
		writeError(w, http.StatusNotImplemented, "saved views are not available on this store")
		return
	}
	var req savedViewRequest
	if err := decodeJSONBodyLimit(r, &req, savedViewBodyLimit); err != nil {
		writeError(w, http.StatusBadRequest, "body must be a JSON object with name, list, filter (string values) and shared")
		return
	}
	caps := rt.savedViewCaps(r.Context())
	req, err := validateSavedView(req, caps)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	c, err := rt.resolveSavedViewCaller(r)
	if err != nil {
		rt.internalError(w, "api: save view: resolve caller", err)
		return
	}
	team := c.homeTeam(r)
	if req.Shared == store.SavedViewTeam && (team == "" || !store.TeamRoleAtLeast(c.roleIn(team), store.TeamRoleDeveloper)) {
		writeError(w, http.StatusForbidden, "sharing a view needs a developer role or higher in your current team")
		return
	}
	prev, err := vs.FindSavedView(r.Context(), c.owner, req.List, req.Name)
	exists := err == nil
	if err != nil && !errors.Is(err, store.ErrSavedViewNotFound) {
		rt.internalError(w, "api: save view: find", err)
		return
	}
	if exists && !c.teamVisible(prev.TeamID) {
		writeError(w, http.StatusNotFound, msgSavedViewNotFound)
		return
	}
	id, err := randomOpaqueID("sv_")
	if err != nil {
		rt.internalError(w, "api: save view: id", err)
		return
	}
	filter, err := json.Marshal(req.Filter)
	if err != nil {
		rt.internalError(w, "api: save view: encode filter", err)
		return
	}
	saved, err := vs.UpsertSavedView(r.Context(), store.SavedView{ID: id, Owner: c.owner, TeamID: team, List: req.List, Name: req.Name, Filter: string(filter), Shared: req.Shared}, caps.perUser)
	if errors.Is(err, store.ErrSavedViewLimit) {
		writeError(w, http.StatusConflict, fmt.Sprintf("you have reached the limit of %d saved views; delete one first", caps.perUser))
		return
	}
	if err != nil {
		rt.internalError(w, "api: save view", err)
		return
	}
	if exists {
		rt.auditSavedView(r, AuditActionSavedViewUpdate, saved)
	} else {
		rt.auditSavedView(r, AuditActionSavedViewCreate, saved)
	}
	if (!exists && saved.Shared == store.SavedViewTeam) || (exists && (prev.TeamID != saved.TeamID || prev.Shared != saved.Shared)) {
		rt.auditSavedView(r, AuditActionSavedViewShare, saved)
	}
	status := http.StatusOK
	if !exists {
		status = http.StatusCreated
	}
	writeJSON(w, status, c.resource(saved))
}

// handleDeleteSavedView handles DELETE /api/v1/saved-views/{id}: the owner, or an admin of the team it is shared with.
func (rt *Router) handleDeleteSavedView(w http.ResponseWriter, r *http.Request) {
	vs := rt.savedViews()
	if vs == nil {
		writeError(w, http.StatusNotFound, msgSavedViewNotFound)
		return
	}
	c, err := rt.resolveSavedViewCaller(r)
	if err != nil {
		rt.internalError(w, "api: delete view: resolve caller", err)
		return
	}
	v, err := vs.GetSavedView(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrSavedViewNotFound) || (err == nil && !c.canRead(v)) {
		writeError(w, http.StatusNotFound, msgSavedViewNotFound)
		return
	}
	if err != nil {
		rt.internalError(w, "api: delete view: get", err)
		return
	}
	if !c.canDelete(v) {
		writeError(w, http.StatusForbidden, "only the owner or a team admin can delete this view")
		return
	}
	if err := vs.DeleteSavedView(r.Context(), v.ID); err != nil {
		if errors.Is(err, store.ErrSavedViewNotFound) {
			writeError(w, http.StatusNotFound, msgSavedViewNotFound)
			return
		}
		rt.internalError(w, "api: delete view", err)
		return
	}
	rt.auditSavedView(r, AuditActionSavedViewDelete, v)
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) auditSavedView(r *http.Request, action string, v store.SavedView) {
	if rt.auditLog == nil {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: saved view audit id failed", slog.String("error", err.Error()))
		return
	}
	detail, _ := json.Marshal(map[string]string{"name": v.Name, "list": v.List, "shared": v.Shared, "team_id": v.TeamID})
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityWrite, Method: r.Method, Path: "/api/v1/saved-views/" + v.ID, StatusCode: http.StatusOK,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action:     action,
		Detail:     string(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: saved view audit failed", slog.String("error", err.Error()), slog.String("view_id", v.ID))
	}
}
