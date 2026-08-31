package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// promotePreviewSide is one side of a promotion preview: the app and the
// image it currently runs. Unlike deployCompareSide there is no
// DeployID/CommitSHA/Status/timestamps to show, source and target are
// both live desired state, not stored deploy attempts.
type promotePreviewSide struct {
	AppName string `json:"app_name"`
	Image   string `json:"image"`
}

// promotePreviewResource is GET .../promote/preview's response shape.
type promotePreviewResource struct {
	SourceApp           string               `json:"source_app"`
	TargetApp           string               `json:"target_app"`
	Environment         environmentResource  `json:"environment"`
	From                promotePreviewSide   `json:"from"`
	To                  promotePreviewSide   `json:"to"`
	Changes             []deployCompareField `json:"changes"`
	UnsnapshottedFields []string             `json:"unsnapshotted_fields"`
	Note                string               `json:"note"`
}

const promotePreviewNote = "Only the image tag is compared here. Environment variables are not diffed: environment-tier env vars are resolved live per environment at deploy time, not snapshotted per app, so there is nothing stale to compare. Ports, domains, resource limits, and other service configuration are the target app's own settings and are left untouched by a promotion."

// handlePromotePreview handles
// GET /api/v1/apps/{name}/promote/preview?to={environmentId}&target={appName}.
// See resolvePromotion for how the target app is found or validated.
func (rt *Router) handlePromotePreview(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	res, ok := rt.resolvePromotion(w, r, name, r.URL.Query().Get("to"), r.URL.Query().Get("target"))
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toPromotePreviewResource(res))
}

func toPromotePreviewResource(res promoteResolution) promotePreviewResource {
	var changes []deployCompareField
	if res.source.Image != res.target.Image {
		changes = append(changes, deployCompareField{Field: "image", From: res.target.Image, To: res.source.Image})
	}
	return promotePreviewResource{
		SourceApp:           res.source.Name,
		TargetApp:           res.target.Name,
		Environment:         toEnvironmentResource(res.env),
		From:                promotePreviewSide{AppName: res.source.Name, Image: res.source.Image},
		To:                  promotePreviewSide{AppName: res.target.Name, Image: res.target.Image},
		Changes:             changes,
		UnsnapshottedFields: unsnapshottedDeployFields,
		Note:                promotePreviewNote,
	}
}

type promoteTriggerRequest struct {
	To      string `json:"to"`
	Target  string `json:"target,omitempty"`
	Confirm bool   `json:"confirm,omitempty"`
}

// handlePromoteApp handles POST /api/v1/apps/{name}/promote: points the
// resolved target app's image at name's current image and redeploys it
// through the exact same setDesiredImage/recordInstantDeployAttempt path
// a plain trigger or rollback uses (deploys.go), rather than a separate
// promotion-specific reconciliation mechanism. Promoting into a protected
// environment requires confirm: true, the same gate handleTriggerDeploy
// enforces (environments.go's environmentNeedsConfirmation).
func (rt *Router) handlePromoteApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req promoteTriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	res, ok := rt.resolvePromotion(w, r, name, req.To, req.Target)
	if !ok {
		return
	}

	if environmentNeedsConfirmation(res.env, req.Confirm) {
		writeEnvironmentConfirmationRequired(w, res.env)
		return
	}

	updated, err := rt.setDesiredImage(r.Context(), res.target, res.source.Image)
	if err != nil {
		rt.logger.Error("api: promote app failed", slog.String("error", err.Error()), slog.String("source", name), slog.String("target", res.target.Name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.recordInstantDeployAttempt(r.Context(), res.target.Name, res.source.Image, store.DeployAttemptSourcePromote)

	writeJSON(w, http.StatusAccepted, toAppResource(updated))
}

// promoteResolution is what resolvePromotion found: the source app, the
// environment being promoted into, and the target app within it.
type promoteResolution struct {
	source store.DesiredService
	target store.DesiredService
	env    store.Environment
}

// resolvePromotion loads sourceName, validates envID, and finds the app
// to promote into: targetName if given, or the sole app in envID tagged
// with it otherwise. Promotion is scoped to one project (repo-plan
// section 5's own scope note for this feature): apps here have no
// declared 1:1 relationship across environments (an app is independently
// named and only optionally tagged with a project/environment), so
// "same project, sibling environment" is the actual constraint enforced,
// not "the same logical app in another environment". Writes the
// 400/404/500 response itself (ok=false) on failure, the same shape
// loadDeployCompareSide already establishes for a two-call-site handler
// pair (preview, trigger).
func (rt *Router) resolvePromotion(w http.ResponseWriter, r *http.Request, sourceName, envID, targetName string) (promoteResolution, bool) {
	source, err := rt.apps.GetDesiredService(r.Context(), sourceName)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return promoteResolution{}, false
	}
	if err != nil {
		rt.logger.Error("api: resolve promotion: load source failed", slog.String("error", err.Error()), slog.String("name", sourceName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return promoteResolution{}, false
	}

	if envID == "" {
		writeError(w, http.StatusBadRequest, "to is required")
		return promoteResolution{}, false
	}
	env, err := rt.environments.GetEnvironment(r.Context(), envID)
	if errors.Is(err, store.ErrEnvironmentNotFound) {
		writeError(w, http.StatusBadRequest, "unknown environment: "+envID)
		return promoteResolution{}, false
	}
	if err != nil {
		rt.logger.Error("api: resolve promotion: load environment failed", slog.String("error", err.Error()), slog.String("environment_id", envID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return promoteResolution{}, false
	}

	if source.ProjectID == "" || source.ProjectID != env.ProjectID {
		writeError(w, http.StatusBadRequest, "app "+sourceName+" is not in the same project as environment "+envID+"; promotion only moves an image within one project")
		return promoteResolution{}, false
	}

	if targetName != "" {
		if targetName == sourceName {
			writeError(w, http.StatusBadRequest, "target app must be different from the source app")
			return promoteResolution{}, false
		}
		target, err := rt.apps.GetDesiredService(r.Context(), targetName)
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "target app not found: "+targetName)
			return promoteResolution{}, false
		}
		if err != nil {
			rt.logger.Error("api: resolve promotion: load target failed", slog.String("error", err.Error()), slog.String("name", targetName))
			writeError(w, http.StatusInternalServerError, "internal error")
			return promoteResolution{}, false
		}
		if target.ProjectID != env.ProjectID {
			writeError(w, http.StatusBadRequest, "target app "+targetName+" is not in the same project as environment "+envID)
			return promoteResolution{}, false
		}
		return promoteResolution{source: *source, target: *target, env: env}, true
	}

	candidates, err := rt.findPromotionCandidates(r.Context(), sourceName, env)
	if err != nil {
		rt.logger.Error("api: resolve promotion: list candidates failed", slog.String("error", err.Error()), slog.String("environment_id", envID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return promoteResolution{}, false
	}
	switch len(candidates) {
	case 0:
		writeError(w, http.StatusNotFound, "no app tagged with environment "+envID+" found in this project; specify ?target=<app-name>")
		return promoteResolution{}, false
	case 1:
		return promoteResolution{source: *source, target: candidates[0], env: env}, true
	default:
		names := make([]string, len(candidates))
		for i, c := range candidates {
			names[i] = c.Name
		}
		writeError(w, http.StatusBadRequest, "multiple apps tagged with environment "+envID+" in this project ("+strings.Join(names, ", ")+"); specify ?target=<app-name>")
		return promoteResolution{}, false
	}
}

// findPromotionCandidates lists every app tagged with env's project and
// ID, excluding sourceName itself: the auto-discovery path resolvePromotion
// falls back to when no explicit target is given. There is deliberately
// no server-side filtered listing endpoint for this (projects.go's own
// doc comment on why project_id filtering stays client-side); filtering
// the full ListDesiredServices result in-memory here is the same
// judgment call, just server-side because both preview and trigger need
// it before responding, not for a UI list render.
func (rt *Router) findPromotionCandidates(ctx context.Context, sourceName string, env store.Environment) ([]store.DesiredService, error) {
	all, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, err
	}
	var out []store.DesiredService
	for _, s := range all {
		if s.Name == sourceName {
			continue
		}
		if s.ProjectID == env.ProjectID && s.EnvironmentID == env.ID {
			out = append(out, s)
		}
	}
	return out, nil
}
