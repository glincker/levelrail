package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	auditActionOneShotStart  = "jobs.run.start"
	auditActionOneShotCancel = "jobs.run.cancel"
	// oneShotAuditCommandMax keeps an audit row small even for a long argv.
	oneShotAuditCommandMax = 512
)

// oneShotGuard tracks per-app active runs (for the concurrency cap) and the
// cancel func of every run this process started.
type oneShotGuard struct {
	mu      sync.Mutex
	active  map[string]int
	cancels map[string]context.CancelFunc
}

var oneShotRuns = &oneShotGuard{active: map[string]int{}, cancels: map[string]context.CancelFunc{}}

// acquire takes a run slot on app, refusing once limit runs are active.
func (g *oneShotGuard) acquire(app string, limit int) (release func(), ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if limit > 0 && g.active[app] >= limit {
		return nil, false
	}
	g.active[app]++
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			if g.active[app]--; g.active[app] <= 0 {
				delete(g.active, app)
			}
		})
	}, true
}

func (g *oneShotGuard) track(id string, cancel context.CancelFunc) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cancels[id] = cancel
}

func (g *oneShotGuard) untrack(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.cancels, id)
}

func (g *oneShotGuard) cancel(id string) bool {
	g.mu.Lock()
	c, ok := g.cancels[id]
	g.mu.Unlock()
	if ok {
		c()
	}
	return ok
}

// clampOneShotTimeout lowers a requested timeout to the runtime ceiling, so a
// caller can never hold a container longer than the operator allows.
func (rt *Router) clampOneShotTimeout(ctx context.Context, seconds int) int {
	ceiling := rt.runtimeSettings.Int(ctx, runtimesettings.KeyAppsRunMaxTimeoutSeconds)
	if ceiling > 0 && (seconds <= 0 || seconds > ceiling) {
		return ceiling
	}
	return seconds
}

func (rt *Router) applyOneShotResourceLimits(ctx context.Context, task *store.ScheduledTask) {
	task.MemoryLimitMB = rt.runtimeSettings.Int(ctx, runtimesettings.KeyAppsRunMemoryLimitMB)
	task.CPULimitMillis = rt.runtimeSettings.Int(ctx, runtimesettings.KeyAppsRunCPULimitMillis)
}

func (rt *Router) acquireOneShotSlot(w http.ResponseWriter, r *http.Request, app string) (func(), bool) {
	limit := rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyAppsRunMaxConcurrentPerApp)
	release, ok := oneShotRuns.acquire(app, limit)
	if !ok {
		writeError(w, http.StatusTooManyRequests, fmt.Sprintf("app %q already has %d one-off runs active (setting %s); wait for one to finish or cancel it", app, limit, runtimesettings.KeyAppsRunMaxConcurrentPerApp))
	}
	return release, ok
}

// handleCancelOneShotJob handles POST /api/v1/jobs/runs/{id}/cancel. Cancelling
// the run context kills the container; the runner removes it either way.
func (rt *Router) handleCancelOneShotJob(w http.ResponseWriter, r *http.Request) {
	oneShotInit()
	id := r.PathValue("id")
	entry, ok := oneShotRegistry.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "one-shot job not found on this control plane")
		return
	}
	if !rt.authorizeRefs(w, r, AbilityWrite, notFoundAs(http.StatusNotFound, "one-shot job not found on this control plane"), appRef(entry.App)) {
		return
	}
	if !oneShotRuns.cancel(id) {
		writeError(w, http.StatusConflict, "one-shot job is not running")
		return
	}
	rt.auditOneShot(r, auditActionOneShotCancel, entry.App, id, nil, http.StatusAccepted)
	writeJSON(w, http.StatusAccepted, toOneShotResponse(entry))
}

func (rt *Router) auditOneShot(r *http.Request, action, app, jobID string, argv []string, status int) {
	if rt.auditLog == nil {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: one-shot audit id failed", slog.String("error", err.Error()))
		return
	}
	detail := fmt.Sprintf("app=%s job=%s", app, jobID)
	if len(argv) > 0 {
		detail += " command=" + redactRunArgv(argv)
	}
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityDeploy, Method: http.MethodPost, Path: "/api/v1/jobs/runs/" + jobID, StatusCode: status,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")), Action: action, Detail: detail,
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: one-shot audit save failed", slog.String("error", err.Error()), slog.String("app", app))
	}
}

const secretWord = `pass|secret|token|key|auth|credential|cookie|session|private|signature`

var (
	secretArgName = regexp.MustCompile(`(?i)(` + secretWord + `)`)
	// Applied inside every arg, so a whole "sh -c" script is covered too.
	inArgSecrets = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(authorization:\s*)(?:(?:bearer|basic|token)\s+)?\S+`),
		regexp.MustCompile(`(?i)(\b[\w.-]*(?:` + secretWord + `)[\w.-]*=)\S+`),
		regexp.MustCompile(`(?i)((?:^|\s)--?[\w-]*(?:` + secretWord + `)[\w-]*\s+)[^\s-]\S*`),
		regexp.MustCompile(`((?:^|\s)-[pu]\s*)[^\s-]\S*`),
		regexp.MustCompile(`(://)[^\s:@/]*:[^\s@]*@`),
	}
	// valueFlags hide the next arg: a header, basic-auth user:pass, or a short password flag.
	valueFlags = map[string]bool{"-H": true, "--header": true, "-u": true, "--user": true, "-p": true}
)

// redactRunArgv renders argv for the audit log with secret-looking values
// hidden: KEY=VAL pairs, values after secret flags, auth headers and URL passwords.
func redactRunArgv(argv []string) string {
	out := make([]string, len(argv))
	hideNext := false
	for i, a := range argv {
		switch {
		case hideNext:
			out[i], hideNext = redactedSecret, false
		case valueFlags[a] || (strings.HasPrefix(a, "-") && !strings.Contains(a, "=") && !strings.ContainsAny(a, " \t") && secretArgName.MatchString(a)):
			out[i], hideNext = a, true
		default:
			for j, re := range inArgSecrets {
				repl := "${1}" + redactedSecret
				if j == len(inArgSecrets)-1 {
					repl += "@"
				}
				a = re.ReplaceAllString(a, repl)
			}
			out[i] = a
		}
	}
	s := strings.Join(out, " ")
	if len(s) > oneShotAuditCommandMax {
		s = s[:oneShotAuditCommandMax] + "..."
	}
	return s
}
