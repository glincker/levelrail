package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// staticSiteHookStore is the optional deploy hook half of the static site store.
type staticSiteHookStore interface {
	GetStaticSiteDeployHook(ctx context.Context, name string) (hash string, createdAt time.Time, err error)
	SetStaticSiteDeployHook(ctx context.Context, name, hash string, createdAt time.Time) error
}

// Audit actions for the Pages deploy hook; the secret is never recorded.
const (
	AuditActionStaticSiteHookCreated   = "static_site.hook_created"
	AuditActionStaticSiteHookRotated   = "static_site.hook_rotated"
	AuditActionStaticSiteHookRevoked   = "static_site.hook_revoked"
	AuditActionStaticSiteHookTriggered = "static_site.hook_triggered"
)

const (
	pagesHookURLPrefix = "/api/v1/hooks/pages/"
	envPagesHookIDWait = "APP_PAGES_HOOK_DEPLOY_ID_WAIT_MS"
	defaultPagesIDWait = 3000
	pagesHookName      = "pages-hook"
)

// staticSiteHookResource never carries the secret except URL on create and rotate, the one time it can be copied.
type staticSiteHookResource struct {
	Enabled   bool       `json:"enabled"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	URL       string     `json:"url,omitempty"`
}

func (rt *Router) staticHookDeps(w http.ResponseWriter, r *http.Request) (string, staticSiteHookStore, bool) {
	hs, ok := rt.staticSites.(staticSiteHookStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "deploy hooks are not configured on this control plane")
		return "", nil, false
	}
	site, _, ok := rt.staticSettingsDeps(w, r)
	if !ok {
		return "", nil, false
	}
	return site.Name, hs, true
}

// handleGetStaticSiteHook handles GET /api/v1/static-sites/{name}/hook: state only, never the secret.
func (rt *Router) handleGetStaticSiteHook(w http.ResponseWriter, r *http.Request) {
	name, hs, ok := rt.staticHookDeps(w, r)
	if !ok {
		return
	}
	hash, created, err := hs.GetStaticSiteDeployHook(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: static site hook: read", err, slog.String("name", name))
		return
	}
	out := staticSiteHookResource{Enabled: hash != ""}
	if out.Enabled && !created.IsZero() {
		out.CreatedAt = &created
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePutStaticSiteHook handles POST /api/v1/static-sites/{name}/hook: create, or rotate when one exists.
func (rt *Router) handlePutStaticSiteHook(w http.ResponseWriter, r *http.Request) {
	name, hs, ok := rt.staticHookDeps(w, r)
	if !ok {
		return
	}
	if _, err := rt.gitSources.GetGitSource(r.Context(), name); errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusConflict, "connect a git source before enabling a deploy hook")
		return
	} else if err != nil {
		rt.internalError(w, "api: static site hook: git source", err, slog.String("name", name))
		return
	}
	prev, _, err := hs.GetStaticSiteDeployHook(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: static site hook: read", err, slog.String("name", name))
		return
	}
	secret, err := randomToken()
	if err != nil {
		rt.internalError(w, "api: static site hook: mint secret", err)
		return
	}
	now := time.Now().UTC()
	if err := hs.SetStaticSiteDeployHook(r.Context(), name, hashToken(secret), now); err != nil {
		rt.internalError(w, "api: static site hook: save", err, slog.String("name", name))
		return
	}
	action, status := AuditActionStaticSiteHookCreated, http.StatusCreated
	if prev != "" {
		action, status = AuditActionStaticSiteHookRotated, http.StatusOK
	}
	rt.auditGitSetting(r, name, action, map[string]any{"site": name})
	path := pagesHookURLPrefix + name + "/" + secret
	url := path
	if base, err := rt.controlPlaneBaseURL(r.Context()); err == nil {
		url = base + path
	}
	writeJSON(w, status, staticSiteHookResource{Enabled: true, CreatedAt: &now, URL: url})
}

// handleDeleteStaticSiteHook handles DELETE /api/v1/static-sites/{name}/hook; revoking twice is not an error.
func (rt *Router) handleDeleteStaticSiteHook(w http.ResponseWriter, r *http.Request) {
	name, hs, ok := rt.staticHookDeps(w, r)
	if !ok {
		return
	}
	if err := hs.SetStaticSiteDeployHook(r.Context(), name, "", time.Time{}); err != nil {
		rt.internalError(w, "api: static site hook: revoke", err, slog.String("name", name))
		return
	}
	rt.auditGitSetting(r, name, AuditActionStaticSiteHookRevoked, map[string]any{"site": name})
	w.WriteHeader(http.StatusNoContent)
}

// siteDeployActive reports a running or queued deploy of name.
func (rt *Router) siteDeployActive(ctx context.Context, name string) bool {
	if rt.deployAttempts == nil {
		return false
	}
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, name)
	if err != nil {
		return false
	}
	for _, a := range attempts {
		if a.Status == store.DeployAttemptStatusRunning || a.Status == store.DeployAttemptStatusQueued {
			return true
		}
	}
	return false
}

// handleStaticSiteHookTrigger is the public hook URL. Unknown site and wrong secret answer the same 404.
func (rt *Router) handleStaticSiteHookTrigger(w http.ResponseWriter, r *http.Request) {
	site, secret := r.PathValue("site"), r.PathValue("secret")
	lim := rt.buildHookLimits()
	if ok, retry := lim.failures.allow("probe:" + clientIP(r)); !ok {
		tooManyBuildHookRequests(w, retry)
		return
	}
	hs, ok := rt.staticSites.(staticSiteHookStore)
	want := ""
	if ok {
		if h, _, err := hs.GetStaticSiteDeployHook(r.Context(), site); err == nil {
			want = h
		}
	}
	if subtle.ConstantTimeCompare([]byte(hashToken(secret)), []byte(want)) != 1 || want == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	gs, err := rt.gitSources.GetGitSource(r.Context(), site)
	if err != nil || gs == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if ok, retry := lim.perHook.allow("pages:" + site); !ok {
		tooManyBuildHookRequests(w, retry)
		return
	}
	if frozen, reason := rt.freezeStatus(r.Context(), site); frozen {
		rt.auditStaticSiteHookTrigger(r, site, http.StatusConflict)
		writeError(w, http.StatusConflict, "deploys are frozen: "+reason)
		return
	}
	if _, busy := lim.pagesInflight.LoadOrStore(site, struct{}{}); busy || rt.siteDeployActive(r.Context(), site) {
		if !busy {
			lim.pagesInflight.Delete(site)
		}
		rt.auditStaticSiteHookTrigger(r, site, http.StatusConflict)
		writeError(w, http.StatusConflict, "a deploy is already running")
		return
	}
	known := map[string]bool{}
	if rt.deployAttempts != nil {
		if prior, err := rt.deployAttempts.ListDeployAttempts(r.Context(), site); err == nil {
			for _, a := range prior {
				known[a.ID] = true
			}
		}
	}
	done := make(chan struct{})
	go func(ctx context.Context) {
		defer close(done)
		defer lim.pagesInflight.Delete(site)
		ctx, cancel := context.WithTimeout(ctx, time.Duration(envInt(envBuildHookTimeout, defaultBuildHookTO))*time.Second)
		defer cancel()
		h := store.BuildHook{ID: pagesHookName + ":" + site, ServiceName: site, Name: pagesHookName}
		if res := rt.executeBuildHook(ctx, h, buildHookPayload{}); len(res) >= 7 && res[:7] == "failed:" {
			rt.logger.Warn("api: pages hook: deploy failed", slog.String("name", site), slog.String("result", res))
		}
	}(context.WithoutCancel(r.Context()))
	rt.auditStaticSiteHookTrigger(r, site, http.StatusAccepted)
	out := map[string]string{"status": "deploy triggered"}
	if id := rt.awaitNewDeployID(r.Context(), site, known, done); id != "" {
		out["deploy_id"] = id
	}
	writeJSON(w, http.StatusAccepted, out)
}

// awaitNewDeployID waits briefly for the attempt row the trigger creates; empty when it is not visible in time.
func (rt *Router) awaitNewDeployID(ctx context.Context, site string, known map[string]bool, done <-chan struct{}) string {
	if rt.deployAttempts == nil {
		return ""
	}
	deadline := time.After(time.Duration(envInt(envPagesHookIDWait, defaultPagesIDWait)) * time.Millisecond)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for finished := false; ; {
		if attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, site); err == nil {
			for _, a := range attempts {
				if !known[a.ID] {
					return a.ID
				}
			}
		}
		if finished {
			return ""
		}
		select {
		case <-done:
			finished = true
		case <-deadline:
			return ""
		case <-ctx.Done():
			return ""
		case <-tick.C:
		}
	}
}

// auditStaticSiteHookTrigger logs the call with the secret redacted from the path.
func (rt *Router) auditStaticSiteHookTrigger(r *http.Request, site string, status int) {
	clone := r.Clone(r.Context())
	u := *r.URL
	u.Path = loggablePath(u.Path)
	clone.URL = &u
	rt.recordAudit(r.Context(), clone, AbilityDeploy, "webhook", site, site+"/"+pagesHookName, status)
}
