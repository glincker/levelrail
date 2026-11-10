package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// BuildHookStore is the store surface for per-app incoming build hooks.
type BuildHookStore interface {
	CreateBuildHook(ctx context.Context, h store.BuildHook) error
	ListBuildHooks(ctx context.Context, serviceName string) ([]store.BuildHook, error)
	GetBuildHook(ctx context.Context, id string) (store.BuildHook, error)
	SetBuildHookTokenHash(ctx context.Context, serviceName, id, hash string) error
	SetBuildHookAllowedCIDRs(ctx context.Context, serviceName, id string, cidrs []string) error
	DeleteBuildHook(ctx context.Context, serviceName, id string) error
	DeleteBuildHooksForService(ctx context.Context, serviceName string) error
	RecordBuildHookTrigger(ctx context.Context, id, result string, at time.Time) error
	RecordBuildHookResult(ctx context.Context, id, result string) error
	RecordBuildHookRefusal(ctx context.Context, id, result string, at time.Time) error
}

const (
	envBuildHooksPerApp = "APP_BUILD_HOOKS_MAX_PER_APP"
	buildHooksPerApp    = 20
	buildHookIDPrefix   = "bh_"
	buildHookURLPrefix  = "/api/v1/hooks/deploy/"
	// buildHookUpdateMaxBody fits the largest allowlist the setting permits with room to spare.
	buildHookUpdateMaxBody = 64 << 10
)

var buildHookNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,62}$`)

type buildHookResource struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Branch          string     `json:"branch,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty"`
	LastResult      string     `json:"last_result,omitempty"`
	TriggerCount    int        `json:"trigger_count"`
	// AllowedCIDRs lists the source ranges that may fire the hook; empty allows any address.
	AllowedCIDRs []string `json:"allowed_cidrs"`
	// LastUsedAt is the latest authenticated call, including refused ones.
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	LastRefusal string     `json:"last_refusal,omitempty"`
	// Path and Token are set only on create and rotate, the one time the secret is visible.
	Path  string `json:"path,omitempty"`
	Token string `json:"token,omitempty"`
}

type createBuildHookRequest struct {
	Name         string   `json:"name"`
	Branch       string   `json:"branch"`
	AllowedCIDRs []string `json:"allowed_cidrs"`
}

// updateBuildHookRequest replaces the allowlist. A missing list is refused; clearing needs allow_any so a stray body never opens a hook.
type updateBuildHookRequest struct {
	AllowedCIDRs *[]string `json:"allowed_cidrs"`
	AllowAny     bool      `json:"allow_any"`
}

func toBuildHookResource(h store.BuildHook) buildHookResource {
	return buildHookResource{ID: h.ID, Name: h.Name, Branch: h.Branch, CreatedAt: h.CreatedAt,
		LastTriggeredAt: h.LastTriggeredAt, LastResult: h.LastResult, TriggerCount: h.TriggerCount,
		AllowedCIDRs: append([]string{}, h.AllowedCIDRs...), LastUsedAt: h.LastUsedAt, LastRefusal: h.LastRefusal}
}

// normalizeBuildHookCIDRs canonicalizes and dedupes the allowlist; a bare address becomes a single-host range.
func (rt *Router) normalizeBuildHookCIDRs(r *http.Request, in []string) ([]string, error) {
	maxN := rt.runtimeSettings.Limit(r.Context(), runtimesettings.KeyBuildHookMaxCIDRs)
	if len(in) > maxN {
		return nil, fmt.Errorf("at most %d allowed_cidrs entries", maxN)
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, raw := range in {
		raw = strings.TrimSpace(raw)
		var p netip.Prefix
		if a, err := netip.ParseAddr(raw); err == nil {
			a = a.Unmap()
			p = netip.PrefixFrom(a, a.BitLen())
		} else if p, err = netip.ParsePrefix(raw); err != nil {
			return nil, fmt.Errorf("invalid CIDR %q", clipBuildHookField(raw, 64))
		}
		if p != p.Masked() {
			return nil, fmt.Errorf("invalid CIDR %q: host bits are set, use %s", p.String(), p.Masked().String())
		}
		if p.Bits() == 0 {
			return nil, fmt.Errorf("invalid CIDR %q: a /0 range allows every source, use allow_any instead", p.String())
		}
		if p.Addr().Is4In6() {
			return nil, fmt.Errorf("invalid CIDR %q: use the IPv4 form", p.String())
		}
		if c := p.String(); !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out, nil
}

func (rt *Router) buildHookApp(w http.ResponseWriter, r *http.Request) (string, bool) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return "", false
	} else if err != nil {
		rt.internalError(w, "api: build hooks: load app", err, slog.String("name", name))
		return "", false
	}
	return name, true
}

func (rt *Router) handleListBuildHooks(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.buildHookApp(w, r)
	if !ok {
		return
	}
	hooks, err := rt.buildHooks.ListBuildHooks(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: list build hooks", err, slog.String("name", name))
		return
	}
	out := make([]buildHookResource, 0, len(hooks))
	for _, h := range hooks {
		out = append(out, toBuildHookResource(h))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateBuildHook mints a hook; the secret URL is returned once and only its hash is kept.
func (rt *Router) handleCreateBuildHook(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.buildHookApp(w, r)
	if !ok {
		return
	}
	var req createBuildHookRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name, req.Branch = strings.TrimSpace(req.Name), strings.TrimSpace(req.Branch)
	if !buildHookNameRE.MatchString(req.Name) {
		writeError(w, http.StatusBadRequest, "name must be 1 to 63 characters: letters, digits, spaces, dot, dash, underscore")
		return
	}
	if strings.ContainsAny(req.Branch, " \t\r\n") || len(req.Branch) > 255 {
		writeError(w, http.StatusBadRequest, "invalid branch")
		return
	}
	cidrs, err := rt.normalizeBuildHookCIDRs(r, req.AllowedCIDRs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	existing, err := rt.buildHooks.ListBuildHooks(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: create build hook: list", err, slog.String("name", name))
		return
	}
	if len(existing) >= envInt(envBuildHooksPerApp, buildHooksPerApp) {
		writeError(w, http.StatusConflict, "build hook limit reached for this app")
		return
	}
	id, err := randomOpaqueID(buildHookIDPrefix)
	if err != nil {
		rt.internalError(w, "api: create build hook: mint id", err)
		return
	}
	token, err := randomToken()
	if err != nil {
		rt.internalError(w, "api: create build hook: mint token", err)
		return
	}
	h := store.BuildHook{ID: id, ServiceName: name, Name: req.Name, Branch: req.Branch, TokenHash: hashToken(token), CreatedAt: time.Now().UTC(), AllowedCIDRs: cidrs}
	if err := rt.buildHooks.CreateBuildHook(r.Context(), h); errors.Is(err, store.ErrBuildHookExists) {
		writeError(w, http.StatusConflict, "a build hook with that name already exists")
		return
	} else if err != nil {
		rt.internalError(w, "api: create build hook", err, slog.String("name", name))
		return
	}
	res := toBuildHookResource(h)
	res.Token, res.Path = token, buildHookURLPrefix+id+"/"+token
	writeJSON(w, http.StatusCreated, res)
}

// handleRotateBuildHook swaps the secret; the old URL stops working at once.
func (rt *Router) handleRotateBuildHook(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.buildHookApp(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	token, err := randomToken()
	if err != nil {
		rt.internalError(w, "api: rotate build hook: mint token", err)
		return
	}
	if err := rt.buildHooks.SetBuildHookTokenHash(r.Context(), name, id, hashToken(token)); errors.Is(err, store.ErrBuildHookNotFound) {
		writeError(w, http.StatusNotFound, "build hook not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: rotate build hook", err, slog.String("name", name))
		return
	}
	h, err := rt.buildHooks.GetBuildHook(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: rotate build hook: reload", err, slog.String("name", name))
		return
	}
	res := toBuildHookResource(h)
	res.Token, res.Path = token, buildHookURLPrefix+id+"/"+token
	writeJSON(w, http.StatusOK, res)
}

// handleUpdateBuildHook replaces a hook's source allowlist; the secret is untouched.
func (rt *Router) handleUpdateBuildHook(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.buildHookApp(w, r)
	if !ok {
		return
	}
	var req updateBuildHookRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, buildHookUpdateMaxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var cidrs []string
	switch {
	case req.AllowAny && req.AllowedCIDRs != nil && len(*req.AllowedCIDRs) > 0:
		writeError(w, http.StatusBadRequest, "send either allowed_cidrs or allow_any, not both")
		return
	case req.AllowAny:
	case req.AllowedCIDRs == nil || len(*req.AllowedCIDRs) == 0:
		writeError(w, http.StatusBadRequest, "allowed_cidrs is required; send allow_any true to allow every source")
		return
	default:
		var err error
		if cidrs, err = rt.normalizeBuildHookCIDRs(r, *req.AllowedCIDRs); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	id := r.PathValue("id")
	before, err := rt.buildHooks.GetBuildHook(r.Context(), id)
	if errors.Is(err, store.ErrBuildHookNotFound) || (err == nil && before.ServiceName != name) {
		writeError(w, http.StatusNotFound, "build hook not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: update build hook: load", err, slog.String("name", name))
		return
	}
	if err := rt.buildHooks.SetBuildHookAllowedCIDRs(r.Context(), name, id, cidrs); errors.Is(err, store.ErrBuildHookNotFound) {
		writeError(w, http.StatusNotFound, "build hook not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: update build hook", err, slog.String("name", name))
		return
	}
	h, err := rt.buildHooks.GetBuildHook(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: update build hook: reload", err, slog.String("name", name))
		return
	}
	rt.auditBuildHookAllowlist(r, h, before.AllowedCIDRs)
	writeJSON(w, http.StatusOK, toBuildHookResource(h))
}

// auditBuildHookAllowlist writes the before and after source lists; it carries CIDRs only, never the secret.
func (rt *Router) auditBuildHookAllowlist(r *http.Request, h store.BuildHook, before []string) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: build hook audit id failed", slog.String("error", err.Error()))
		return
	}
	a := rt.accessActor(r)
	detail, _ := json.Marshal(map[string]any{"app": h.ServiceName, "hook": h.ID,
		"before": append([]string{}, before...), "after": append([]string{}, h.AllowedCIDRs...)})
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name, Ability: AbilityWriteSensitive,
		Method: http.MethodPatch, Path: r.URL.Path, StatusCode: http.StatusOK, CreatedAt: store.FormatAuditTime(time.Now()),
		RemoteAddr: clientIP(r), ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action: "app.build_hook.allowlist", Detail: string(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: build hook audit failed", slog.String("error", err.Error()))
	}
}

func (rt *Router) handleRevokeBuildHook(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.buildHookApp(w, r)
	if !ok {
		return
	}
	if err := rt.buildHooks.DeleteBuildHook(r.Context(), name, r.PathValue("id")); errors.Is(err, store.ErrBuildHookNotFound) {
		writeError(w, http.StatusNotFound, "build hook not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: revoke build hook", err, slog.String("name", name))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
