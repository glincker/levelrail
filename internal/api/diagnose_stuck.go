package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/diskguard"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/standalone"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/stuck"
)

const (
	envStuckThreshold     = "APP_STUCK_THRESHOLD"
	defaultStuckThreshold = 5 * time.Minute
)

// stuckResourceFromPath scopes the read to the app or database named in the path.
func stuckResourceFromPath(r *http.Request) string {
	switch r.PathValue("kind") {
	case stuck.KindApp:
		return "app:" + r.PathValue("name")
	case stuck.KindDatabase:
		return "database:" + r.PathValue("name")
	}
	return ""
}

// handleDiagnoseStuck handles GET /api/v1/diagnose/{kind}/{name}: the
// deterministic "why is this not running" explanation for an app, database,
// domain or node, built from its reconcile conditions plus nearby context.
func (rt *Router) handleDiagnoseStuck(w http.ResponseWriter, r *http.Request) {
	kind, name := r.PathValue("kind"), r.PathValue("name")
	ctx := r.Context()
	in := stuck.Input{
		Kind: kind, Name: name, Now: time.Now(),
		Threshold: envDurationOr(envStuckThreshold, defaultStuckThreshold),
		Ctx:       stuck.Context{Href: map[string]string{}},
	}
	var controller, nodeID string
	switch kind {
	case stuck.KindApp:
		svc, err := rt.apps.GetDesiredService(ctx, name)
		if err != nil {
			rt.stuckLoadError(w, "app", err)
			return
		}
		controller, nodeID = applicationControllerName(name), svc.NodeID
		rt.fillAppStuckContext(ctx, &in, name)
	case stuck.KindDatabase:
		db, err := rt.databases.GetDesiredDatabase(ctx, name)
		if err != nil {
			rt.stuckLoadError(w, "database", err)
			return
		}
		controller, nodeID = databaseControllerName(name), db.NodeID
		in.Ctx.Href["logs"] = "/databases/" + url.PathEscape(name) + "/logs"
	case stuck.KindNode:
		if _, err := rt.nodes.GetNode(ctx, name); err != nil {
			rt.stuckLoadError(w, "node", err)
			return
		}
		controller, nodeID = nodeHealthControllerName(name), name
	case stuck.KindDomain:
		app, ok := rt.stuckDomainOwner(w, r, strings.ToLower(strings.TrimSuffix(name, ".")))
		if !ok {
			return
		}
		controller = applicationControllerName(app)
		if svc, err := rt.apps.GetDesiredService(ctx, app); err == nil {
			nodeID = svc.NodeID
		}
		rt.fillAppStuckContext(ctx, &in, app)
		in.Ctx.Href["logs"] = "/domains/" + url.PathEscape(name) + "?app=" + url.QueryEscape(app)
	default:
		writeError(w, http.StatusBadRequest, "kind must be app, database, domain or node")
		return
	}

	conditions, err := rt.deploys.GetConditions(ctx, controller)
	if err != nil {
		rt.internalError(w, "api: diagnose stuck: load conditions failed", err)
		return
	}
	in.Conditions = stuckConditions(conditions)
	rt.fillStuckNodeAndDisk(ctx, &in, nodeID)
	writeJSON(w, http.StatusOK, stuck.Explain(in))
}

func (rt *Router) stuckLoadError(w http.ResponseWriter, what string, err error) {
	if isNotFound(err) {
		writeError(w, http.StatusNotFound, what+" not found")
		return
	}
	rt.internalError(w, "api: diagnose stuck: load "+what+" failed", err)
}

func isNotFound(err error) bool {
	return errors.Is(err, store.ErrServiceNotFound) || errors.Is(err, store.ErrNodeNotFound) || errors.Is(err, store.ErrDatabaseNotFound)
}

func (rt *Router) stuckDomainOwner(w http.ResponseWriter, r *http.Request, domain string) (string, bool) {
	canSee, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: diagnose stuck: visibility", err)
		return "", false
	}
	owners, err := rt.domainOwners(r.Context())
	if err != nil {
		rt.internalError(w, "api: diagnose stuck: owners", err)
		return "", false
	}
	for _, app := range owners[domain] {
		if canSee(app) && app != standalone.Scope {
			return app, true
		}
	}
	writeError(w, http.StatusNotFound, "domain not found; add it to an app first")
	return "", false
}

func (rt *Router) fillAppStuckContext(ctx context.Context, in *stuck.Input, app string) {
	in.Ctx.Href["logs"] = "/apps/" + url.PathEscape(app) + "/logs"
	in.Ctx.Href["environment"] = "/apps/" + url.PathEscape(app) + "/environment"
	if attempt, err := rt.resolveDiagnosisAttempt(ctx, app, ""); err == nil && attempt != nil && attempt.Error != "" {
		in.Ctx.LastError = attempt.Error
	}
	if c := rt.diagnoseCrashloop(ctx, app); c != nil && c.Firing {
		in.Ctx.Crashloop = true
	}
}

func (rt *Router) fillStuckNodeAndDisk(ctx context.Context, in *stuck.Input, nodeID string) {
	if nodeID != "" && rt.nodes != nil {
		if n, err := rt.nodes.GetNode(ctx, nodeID); err == nil && n != nil {
			in.Ctx.NodeStatus = string(n.Status)
			in.Ctx.Href["node"] = "/nodes/" + url.PathEscape(nodeID)
		}
	}
	if rt.diskGuard != nil {
		if st := rt.diskGuard.Status(); st.Worst == diskguard.LevelCritical {
			in.Ctx.DiskCritical = true
			for _, v := range st.Volumes {
				if v.Level == diskguard.LevelCritical {
					in.Ctx.DiskDetail = fmt.Sprintf("%s has %d MiB free of %d MiB", v.Path, v.FreeBytes>>20, v.TotalBytes>>20)
					break
				}
			}
		}
	}
}

func stuckConditions(cs []reconcile.Condition) []stuck.Condition {
	out := make([]stuck.Condition, 0, len(cs))
	for _, c := range cs {
		out = append(out, stuck.Condition{Type: c.Type, Status: string(c.Status), Reason: c.Reason, Message: c.Message, Since: c.LastTransitionTime})
	}
	return out
}
