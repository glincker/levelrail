package api

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const fleetTimelineDefaultWindow = 24 * time.Hour

// fleetTimelineBar is one deploy on the fleet timeline: only what a bar and its
// hover card need, so a window of hundreds of deploys stays small.
type fleetTimelineBar struct {
	ID            string     `json:"id"`
	App           string     `json:"app"`
	Status        string     `json:"status"`
	Trigger       string     `json:"trigger"`
	Environment   string     `json:"environment"`
	CommitSHA     string     `json:"commit_sha"`
	CommitMessage string     `json:"commit_message"`
	Author        string     `json:"author"`
	Strategy      string     `json:"strategy"`
	ProjectID     string     `json:"project_id"`
	NodeID        string     `json:"node_id"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at"`
	DurationMS    *int64     `json:"duration_ms"`
	RollbackOf    *string    `json:"rollback_of"`
	IsLive        bool       `json:"is_live"`
}

type fleetTimelineResponse struct {
	Since      time.Time          `json:"since"`
	Until      time.Time          `json:"until"`
	Items      []fleetTimelineBar `json:"items"`
	NextCursor string             `json:"next_cursor"`
}

// fleetTimelineScope resolves the app set for the project and node filters,
// intersected with what the caller can read. scoped is false when neither
// filter nor visibility narrows the set.
func (rt *Router) fleetTimelineScope(r *http.Request, q url.Values, visible []string) (apps []string, byApp map[string]store.DesiredService, scoped bool, err error) {
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		return nil, nil, false, err
	}
	byApp = make(map[string]store.DesiredService, len(svcs))
	for _, s := range svcs {
		byApp[s.Name] = s
	}
	project, node := q.Get("project"), q.Get("node")
	if project == "" && node == "" {
		return visible, byApp, visible != nil, nil
	}
	allowed := map[string]bool{}
	for _, n := range visible {
		allowed[n] = true
	}
	apps = []string{}
	for _, s := range svcs {
		if visible != nil && !allowed[s.Name] {
			continue
		}
		if (project != "" && s.ProjectID != project) || (node != "" && s.NodeID != node) {
			continue
		}
		apps = append(apps, s.Name)
	}
	return apps, byApp, true, nil
}

// handleDeploymentsTimeline handles GET /api/v1/deployments/timeline:
// deploys inside a time window, newest first, cursor paginated, narrowed by
// app, project, node and status.
func (rt *Router) handleDeploymentsTimeline(w http.ResponseWriter, r *http.Request) {
	ds, ok := rt.deployAttempts.(deploymentStore)
	if !ok {
		rt.internalError(w, "api: deployments timeline", errNoDeploymentStore)
		return
	}
	now := time.Now().UTC()
	q := r.URL.Query()
	f, err := parseDeploymentQuery(q, now)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if f.Since.IsZero() {
		f.Since = now.Add(-fleetTimelineDefaultWindow)
	}
	if f.Until.IsZero() {
		f.Until = now
	}
	if !f.Since.Before(f.Until) || f.Until.Sub(f.Since) > deploymentsMaxWindow {
		writeError(w, http.StatusBadRequest, "since must be before until, and the window at most 30d")
		return
	}
	visible, err := rt.visibleAppNames(r)
	if err != nil {
		rt.internalError(w, "api: deployments timeline: visibility", err)
		return
	}
	apps, byApp, scoped, err := rt.fleetTimelineScope(r, q, visible)
	if err != nil {
		rt.internalError(w, "api: deployments timeline: scope", err)
		return
	}
	out := fleetTimelineResponse{Since: f.Since, Until: f.Until, Items: []fleetTimelineBar{}}
	if scoped {
		if len(apps) == 0 {
			writeJSON(w, http.StatusOK, out)
			return
		}
		f.VisibleApps = apps
	}
	rows, next, err := ds.ListDeployments(r.Context(), f)
	if errors.Is(err, store.ErrInvalidDeploymentCursor) {
		writeError(w, http.StatusBadRequest, "cursor is not valid")
		return
	}
	if err != nil {
		rt.internalError(w, "api: deployments timeline failed", err)
		return
	}
	out.NextCursor = next
	for _, d := range rows {
		a := d.Attempt
		bar := fleetTimelineBar{
			ID: a.ID, App: a.ServiceName, Status: d.Status, Trigger: d.Trigger, Environment: d.Environment,
			CommitSHA: a.CommitSHA, CommitMessage: firstLine(a.CommitMessage, 200), Author: a.Author,
			Strategy: a.Snapshot.Strategy, StartedAt: a.StartedAt, FinishedAt: a.FinishedAt,
			RollbackOf: strOrNil(d.RollbackOf), IsLive: d.IsLive,
		}
		if svc, found := byApp[a.ServiceName]; found {
			bar.ProjectID, bar.NodeID = svc.ProjectID, svc.NodeID
		}
		if a.FinishedAt != nil {
			ms := a.FinishedAt.Sub(a.StartedAt).Milliseconds()
			bar.DurationMS = &ms
		}
		out.Items = append(out.Items, bar)
	}
	writeJSON(w, http.StatusOK, out)
}
