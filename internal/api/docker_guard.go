package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/dockerguard"
	"github.com/GLINCKER/levelrail/internal/store"
)

// dockerGuardAuditScan caps how many guard audit rows one summary reads.
const dockerGuardAuditScan = 2000

// DockerGuardController is the Docker API guard as the API sees it;
// *dockerguard.Controller satisfies it.
type DockerGuardController interface {
	Status() dockerguard.Status
	SetMode(mode dockerguard.Mode, actor string, now time.Time) (dockerguard.Status, error)
}

// WithDockerGuard enables GET/PUT /api/v1/system/docker-guard, its doctor
// check and its attention item. Without it those report the guard as unknown.
func WithDockerGuard(c DockerGuardController) Option {
	return func(rt *Router) { rt.dockerGuard = c }
}

// SetDockerGuard is WithDockerGuard for a guard booted before the router.
func (rt *Router) SetDockerGuard(c DockerGuardController) { rt.dockerGuard = c }

// dockerGuardRuleSummary is one rule's audit-log history over the window.
type dockerGuardRuleSummary struct {
	Rule      string `json:"rule"`
	Denied    int    `json:"denied"`
	WouldDeny int    `json:"would_deny"`
	LastSeen  string `json:"last_seen"`
	LastPath  string `json:"last_path"`
}

type dockerGuardResource struct {
	dockerguard.Status
	Configured    bool                     `json:"configured"`
	WindowSeconds int64                    `json:"window_seconds"`
	Window        []dockerGuardRuleSummary `json:"window"`
	WouldDeny     int                      `json:"would_deny_total"`
	Denied        int                      `json:"denied_total"`
	// ReadyToEnforce: audit mode has watched a full window with no
	// would-be denials, so switching is unlikely to break anything.
	ReadyToEnforce bool `json:"ready_to_enforce"`
}

type updateDockerGuardRequest struct {
	Mode string `json:"mode"`
}

func (rt *Router) handleGetDockerGuard(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, rt.dockerGuardResource(r.Context(), time.Now()))
}

func (rt *Router) handleUpdateDockerGuard(w http.ResponseWriter, r *http.Request) {
	if rt.dockerGuard == nil {
		writeError(w, http.StatusNotImplemented, "the docker guard is not configured on this control plane")
		return
	}
	var req updateDockerGuardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	mode, err := dockerguard.ParseMode(req.Mode)
	if err != nil || req.Mode == "" {
		writeError(w, http.StatusBadRequest, "mode must be one of off, audit, enforce")
		return
	}
	_, _, actorName, _ := rt.currentActor(r)
	if _, err := rt.dockerGuard.SetMode(mode, actorName, time.Now()); err != nil {
		if errors.Is(err, dockerguard.ErrPinnedByEnv) {
			writeError(w, http.StatusConflict, "mode is pinned by "+dockerguard.EnvMode+"; unset it to manage the guard here")
			return
		}
		rt.internalError(w, "api: update docker guard failed", err)
		return
	}
	rt.handleGetDockerGuard(w, r)
}

func (rt *Router) dockerGuardResource(ctx context.Context, now time.Time) dockerGuardResource {
	tun, _ := dockerguard.TunablesFromEnv(nil)
	res := dockerGuardResource{WindowSeconds: int64(tun.SummaryWindow / time.Second), Window: []dockerGuardRuleSummary{}}
	if rt.dockerGuard == nil {
		res.Status = dockerguard.Status{Mode: dockerguard.ModeOff, Effective: dockerguard.ModeOff, Rules: dockerguard.AllRules, SinceBoot: []dockerguard.RuleCount{}}
		return res
	}
	res.Configured = true
	res.Status = rt.dockerGuard.Status()
	res.Window = rt.dockerGuardWindow(ctx, now.Add(-tun.SummaryWindow))
	for _, s := range res.Window {
		res.WouldDeny += s.WouldDeny
		res.Denied += s.Denied
	}
	observedFor := now.Sub(res.AuditSince)
	res.ReadyToEnforce = res.Effective == dockerguard.ModeAudit && res.Running && res.WouldDeny == 0 &&
		!res.AuditSince.IsZero() && observedFor >= tun.SummaryWindow
	return res
}

// dockerGuardWindow folds the guard's audit rows since cutoff into one
// summary per rule, most frequent first.
func (rt *Router) dockerGuardWindow(ctx context.Context, cutoff time.Time) []dockerGuardRuleSummary {
	out := []dockerGuardRuleSummary{}
	if rt.auditLog == nil {
		return out
	}
	entries, err := rt.auditLog.ListAuditEntries(ctx, dockerGuardAuditScan, nil, store.AuditEntryFilter{Action: dockerguard.ActionFamily})
	if err != nil {
		rt.logger.Warn("api: docker guard summary: list audit entries failed", slog.String("error", err.Error()))
		return out
	}
	byRule := map[string]*dockerGuardRuleSummary{}
	floor := store.FormatAuditTime(cutoff)
	for _, e := range entries {
		if e.CreatedAt < floor {
			continue
		}
		s, ok := byRule[e.Ability]
		if !ok {
			s = &dockerGuardRuleSummary{Rule: e.Ability, LastSeen: e.CreatedAt, LastPath: e.Method + " " + e.Path}
			byRule[e.Ability] = s
		}
		if e.Action == dockerguard.ActionDenied {
			s.Denied++
		} else {
			s.WouldDeny++
		}
	}
	for _, s := range byRule {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if a, b := out[i].Denied+out[i].WouldDeny, out[j].Denied+out[j].WouldDeny; a != b {
			return a > b
		}
		return out[i].Rule < out[j].Rule
	})
	return out
}

// DockerGuardAuditEntry turns a guard decision into an audit_log row. The
// path carries the container name, never the request body.
func DockerGuardAuditEntry(d dockerguard.Decision) (store.AuditEntry, error) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		return store.AuditEntry{}, err
	}
	path := "docker:" + d.Path
	if d.Container != "" {
		path += "?name=" + d.Container
	}
	status := http.StatusOK
	if d.Denied {
		status = http.StatusForbidden
	}
	return store.AuditEntry{
		ID: id, ActorType: "system", ActorID: "docker-guard", ActorName: "docker-guard",
		Ability: d.Rule(), Method: d.Method, Path: path, StatusCode: status,
		CreatedAt: store.FormatAuditTime(d.At), ClientKind: "system", Action: d.Action(),
	}, nil
}
