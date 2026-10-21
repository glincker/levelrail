package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envBuildHookRate      = "APP_BUILD_HOOK_TRIGGERS_PER_MINUTE"
	envBuildHookFailures  = "APP_BUILD_HOOK_AUTH_FAILURES_PER_MINUTE"
	envBuildHookFieldMax  = "APP_BUILD_HOOK_FIELD_MAX_BYTES"
	envBuildHookBodyMax   = "APP_BUILD_HOOK_BODY_MAX_BYTES"
	envBuildHookTimeout   = "APP_BUILD_HOOK_TIMEOUT_SECONDS"
	defaultBuildHookRate  = 6
	defaultBuildHookFails = 30
	defaultBuildHookField = 1024
	defaultBuildHookBody  = 64 * 1024
	defaultBuildHookTO    = 30 * 60
)

// Build args a hook trigger passes to the build; a Dockerfile reads them through ARG.
const (
	buildHookArgTitle  = "TRIGGER_TITLE"
	buildHookArgBody   = "TRIGGER_BODY"
	buildHookArgBranch = "TRIGGER_BRANCH"
	buildHookArgHook   = "TRIGGER_HOOK"
)

type buildArgsCtxKey struct{}

func withExtraBuildArgs(ctx context.Context, args map[string]string) context.Context {
	return context.WithValue(ctx, buildArgsCtxKey{}, args)
}

func extraBuildArgsFrom(ctx context.Context) map[string]string {
	args, _ := ctx.Value(buildArgsCtxKey{}).(map[string]string)
	return args
}

type buildHookLimiters struct {
	once     sync.Once
	perHook  *apiRateLimiter
	failures *apiRateLimiter
}

func (rt *Router) buildHookLimits() *buildHookLimiters {
	l := &rt.buildHookLimiters
	l.once.Do(func() {
		l.perHook = newWindowRateLimiter(envInt(envBuildHookRate, defaultBuildHookRate), time.Minute)
		l.failures = newWindowRateLimiter(envInt(envBuildHookFailures, defaultBuildHookFails), time.Minute)
	})
	return l
}

type buildHookPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// readBuildHookPayload takes title and body from query or a JSON body; any other body is ignored so a CMS can post whatever it likes.
func readBuildHookPayload(r *http.Request) buildHookPayload {
	p := buildHookPayload{Title: r.URL.Query().Get("title"), Body: r.URL.Query().Get("body")}
	if r.Body != nil {
		raw, _ := io.ReadAll(io.LimitReader(r.Body, int64(envInt(envBuildHookBodyMax, defaultBuildHookBody))))
		var j buildHookPayload
		if json.Unmarshal(raw, &j) == nil {
			if j.Title != "" {
				p.Title = j.Title
			}
			if j.Body != "" {
				p.Body = j.Body
			}
		}
	}
	limit := envInt(envBuildHookFieldMax, defaultBuildHookField)
	p.Title, p.Body = clipBuildHookField(p.Title, limit), clipBuildHookField(p.Body, limit)
	return p
}

func clipBuildHookField(s string, limit int) string {
	s = strings.Map(func(r rune) rune {
		if r < ' ' && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
	if len(s) > limit {
		s = strings.ToValidUTF8(s[:limit], "")
	}
	return s
}

// handleBuildHookTrigger is the public hook URL. Unknown id and wrong token answer the same 404 so the URL leaks nothing.
func (rt *Router) handleBuildHookTrigger(w http.ResponseWriter, r *http.Request) {
	id, token := r.PathValue("id"), r.PathValue("token")
	lim := rt.buildHookLimits()
	ip := clientIP(r)
	if ok, retry := lim.failures.allow("probe:" + ip); !ok {
		tooManyBuildHookRequests(w, retry)
		return
	}
	hook, err := rt.buildHooks.GetBuildHook(r.Context(), id)
	want := hook.TokenHash
	if err != nil {
		want = ""
	}
	got := hashToken(token)
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 || want == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if ok, retry := lim.perHook.allow(id); !ok {
		tooManyBuildHookRequests(w, retry)
		return
	}
	payload := readBuildHookPayload(r)
	rt.auditBuildHookTrigger(r, hook)
	if err := rt.buildHooks.RecordBuildHookTrigger(r.Context(), id, "triggered", time.Now()); err != nil {
		rt.logger.Warn("api: build hook: record trigger failed", slog.String("hook_id", id), slog.String("error", err.Error()))
	}
	go rt.runBuildHook(context.WithoutCancel(r.Context()), hook, payload)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "build triggered"})
}

func tooManyBuildHookRequests(w http.ResponseWriter, retry time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
	writeError(w, http.StatusTooManyRequests, "too many requests")
}

// auditBuildHookTrigger logs the call with the secret redacted from the path.
func (rt *Router) auditBuildHookTrigger(r *http.Request, h store.BuildHook) {
	clone := r.Clone(r.Context())
	u := *r.URL
	u.Path = loggablePath(u.Path)
	clone.URL = &u
	rt.recordAudit(r.Context(), clone, AbilityDeploy, "webhook", h.ID, h.ServiceName+"/"+h.Name, http.StatusAccepted)
}

// runBuildHook deploys the app's git source and stores the outcome on the hook.
func (rt *Router) runBuildHook(ctx context.Context, h store.BuildHook, p buildHookPayload) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(envInt(envBuildHookTimeout, defaultBuildHookTO))*time.Second)
	defer cancel()
	result := rt.executeBuildHook(ctx, h, p)
	if err := rt.buildHooks.RecordBuildHookResult(ctx, h.ID, result); err != nil {
		rt.logger.Warn("api: build hook: record result failed", slog.String("hook_id", h.ID), slog.String("error", err.Error()))
	}
}

func (rt *Router) executeBuildHook(ctx context.Context, h store.BuildHook, p buildHookPayload) string {
	if frozen, reason := rt.freezeStatus(ctx, h.ServiceName); frozen {
		return "skipped: deploy freeze active: " + reason
	}
	branch := h.Branch
	if branch == "" {
		gs, err := rt.gitSources.GetGitSource(ctx, h.ServiceName)
		if err != nil {
			return "failed: no git source connected"
		}
		branch = gs.Branch
	}
	args := map[string]string{buildHookArgBranch: branch, buildHookArgHook: h.Name}
	if p.Title != "" {
		args[buildHookArgTitle] = p.Title
	}
	if p.Body != "" {
		args[buildHookArgBody] = p.Body
	}
	msg, err := rt.triggerGitDeploy(withExtraBuildArgs(ctx, args), h.ServiceName, branch, store.DeployAttemptSourceWebhook)
	if err != nil {
		rt.logger.Warn("api: build hook: deploy failed", slog.String("hook_id", h.ID), slog.String("name", h.ServiceName), slog.String("error", err.Error()))
		return "failed: " + err.Error()
	}
	if msg == "" {
		return "deployed"
	}
	return "deployed: " + strings.TrimSpace(msg)
}

// mergeBuildArgs layers hook-supplied args over the app's own, copying so the shared spec map is never mutated.
func mergeBuildArgs(base, extra map[string]string) map[string]string {
	if len(extra) == 0 {
		return base
	}
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
