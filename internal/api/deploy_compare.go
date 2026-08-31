package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// deployCompareSide is one side of a deploy comparison: either a real
// deploy attempt (DeployID set) or the app's current live desired state
// (IsCurrent true, DeployID empty). The current side only ever carries
// Image, because DesiredService is live state, not a historical record:
// there is no CommitSHA/Source/Status/timestamps to show for "now".
type deployCompareSide struct {
	DeployID   string     `json:"deploy_id,omitempty"`
	IsCurrent  bool       `json:"is_current"`
	Image      string     `json:"image"`
	CommitSHA  string     `json:"commit_sha,omitempty"`
	Source     string     `json:"source,omitempty"`
	Status     string     `json:"status,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// deployCompareField is one field that differs between From and To.
type deployCompareField struct {
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// deployCompareResource is GET .../deploys/compare's response shape.
type deployCompareResource struct {
	ServiceName         string               `json:"service_name"`
	From                deployCompareSide    `json:"from"`
	To                  deployCompareSide    `json:"to"`
	Changes             []deployCompareField `json:"changes"`
	UnsnapshottedFields []string             `json:"unsnapshotted_fields"`
	Note                string               `json:"note"`
}

// unsnapshottedDeployFields lists DesiredService fields store.DeployAttempt
// never captures a per-attempt copy of (see that type's own doc comment:
// Image, CommitSHA, Source, Status, and timestamps are the whole of it).
// Named once so the handler's response and its own doc comment can't
// drift apart.
var unsnapshottedDeployFields = []string{
	"env", "port", "host_port", "domains", "resources", "health",
	"replicas", "strategy", "volumes", "labels",
}

const deployCompareUnsnapshottedNote = "Deploy attempts only record the image tag, commit, trigger source, and outcome at trigger time. Environment variables, ports, domains, resource limits, and other service configuration are not snapshotted per attempt, so they cannot be diffed across past deploys, only the app's current live values are known."

// handleCompareDeploys handles
// GET /api/v1/apps/{name}/deploys/compare?from={deployId}&to={deployId}.
// from is required; to is optional and, when omitted, compares from
// against the app's current live desired state. See
// deployCompareUnsnapshottedNote for why the diff is limited to the
// handful of fields store.DeployAttempt actually captures rather than a
// fabricated full config diff.
func (rt *Router) handleCompareDeploys(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: compare deploys: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	fromID := r.URL.Query().Get("from")
	toID := r.URL.Query().Get("to")
	if fromID == "" {
		writeError(w, http.StatusBadRequest, "from is required")
		return
	}

	fromSide, ok := rt.loadDeployCompareSide(w, r, name, fromID)
	if !ok {
		return
	}

	toSide := deployCompareSide{IsCurrent: true, Image: svc.Image}
	if toID != "" {
		toSide, ok = rt.loadDeployCompareSide(w, r, name, toID)
		if !ok {
			return
		}
	}

	writeJSON(w, http.StatusOK, deployCompareResource{
		ServiceName:         name,
		From:                fromSide,
		To:                  toSide,
		Changes:             diffDeployCompareSides(fromSide, toSide),
		UnsnapshottedFields: unsnapshottedDeployFields,
		Note:                deployCompareUnsnapshottedNote,
	})
}

// loadDeployCompareSide loads one deploy attempt by id, scoped to
// appName, writing the 404/500 response itself (ok=false) on failure so
// handleCompareDeploys's two call sites (from, to) share one error path.
func (rt *Router) loadDeployCompareSide(w http.ResponseWriter, r *http.Request, appName, deployID string) (side deployCompareSide, ok bool) {
	a, err := rt.deployAttempts.GetDeployAttempt(r.Context(), deployID)
	if errors.Is(err, store.ErrDeployAttemptNotFound) {
		writeError(w, http.StatusNotFound, "deploy attempt not found: "+deployID)
		return deployCompareSide{}, false
	}
	if err != nil {
		rt.logger.Error("api: compare deploys: load attempt failed", slog.String("error", err.Error()), slog.String("deploy_id", deployID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return deployCompareSide{}, false
	}
	if a.ServiceName != appName {
		writeError(w, http.StatusNotFound, "deploy attempt not found: "+deployID)
		return deployCompareSide{}, false
	}
	return deployCompareSide{
		DeployID:   a.ID,
		Image:      a.Image,
		CommitSHA:  a.CommitSHA,
		Source:     a.Source,
		Status:     a.Status,
		StartedAt:  &a.StartedAt,
		FinishedAt: a.FinishedAt,
	}, true
}

// diffDeployCompareSides lists the captured fields that actually differ
// between from and to. Source is only compared between two real attempts:
// the current side carries no Source, so including it there would read as
// "source changed to empty" rather than "unknown".
func diffDeployCompareSides(from, to deployCompareSide) []deployCompareField {
	var changes []deployCompareField
	if from.Image != to.Image {
		changes = append(changes, deployCompareField{Field: "image", From: from.Image, To: to.Image})
	}
	if from.CommitSHA != to.CommitSHA {
		changes = append(changes, deployCompareField{Field: "commit_sha", From: from.CommitSHA, To: to.CommitSHA})
	}
	if !to.IsCurrent && from.Source != to.Source {
		changes = append(changes, deployCompareField{Field: "source", From: from.Source, To: to.Source})
	}
	return changes
}
