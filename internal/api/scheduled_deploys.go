package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/policy"
	"github.com/GLINCKER/levelrail/internal/scheduledeploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultScheduledDeployListLimit = 50
	maxScheduledDeployListLimit     = 200

	// AppEventScheduledDeploy marks a scheduled deploy being created, canceled, skipped or missed.
	appEventScheduledDeploy = "scheduled_deploy"
)

// ScheduledDeployStore is the store surface the scheduled deploy handlers need.
// *store.DB satisfies it.
type ScheduledDeployStore interface {
	CreateScheduledDeploy(ctx context.Context, d store.ScheduledDeploy) (store.ScheduledDeploy, error)
	GetScheduledDeploy(ctx context.Context, serviceName, id string) (*store.ScheduledDeploy, error)
	ListScheduledDeploys(ctx context.Context, serviceName string, statuses []string, limit int) ([]store.ScheduledDeploy, error)
	CancelScheduledDeploy(ctx context.Context, serviceName, id string, at time.Time) (*store.ScheduledDeploy, error)
}

type scheduledDeployResource struct {
	ID          string     `json:"id"`
	ServiceName string     `json:"service_name"`
	Kind        string     `json:"kind"`
	Image       string     `json:"image,omitempty"`
	Ref         string     `json:"ref,omitempty"`
	RunAt       time.Time  `json:"run_at"`
	Status      string     `json:"status"`
	Reason      string     `json:"reason,omitempty"`
	ApprovalID  string     `json:"approval_id,omitempty"`
	CreatedBy   string     `json:"created_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

func toScheduledDeployResource(d store.ScheduledDeploy) scheduledDeployResource {
	return scheduledDeployResource{
		ID: d.ID, ServiceName: d.ServiceName, Kind: d.Kind, Image: d.Image, Ref: d.Ref, RunAt: d.RunAt,
		Status: d.Status, Reason: d.Reason, ApprovalID: d.ApprovalID, CreatedBy: d.CreatedByName,
		CreatedAt: d.CreatedAt, FinishedAt: d.FinishedAt,
	}
}

type createScheduledDeployRequest struct {
	RunAt string `json:"run_at"`
	Image string `json:"image"`
	Ref   string `json:"ref"`
}

// handleCreateScheduledDeploy handles POST /api/v1/apps/{name}/scheduled-deploys:
// an image or git ref deploy that runs once at run_at.
func (rt *Router) handleCreateScheduledDeploy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: create scheduled deploy: load app failed", err, slog.String("name", name))
		return
	}
	var req createScheduledDeployRequest
	if !decodeReq(w, r, &req) {
		return
	}
	req.Image, req.Ref = strings.TrimSpace(req.Image), strings.TrimSpace(req.Ref)
	d := store.ScheduledDeploy{ServiceName: name}
	switch {
	case req.Image != "" && req.Ref != "":
		writeError(w, http.StatusBadRequest, "set exactly one of image or ref, not both")
		return
	case req.Image == "" && req.Ref == "":
		writeError(w, http.StatusBadRequest, "one of image or ref is required")
		return
	case req.Image != "":
		if err := validateImageReference(req.Image); err != nil {
			writeCodedError(w, http.StatusBadRequest, errCodeInvalidImageRef, "image: "+err.Error())
			return
		}
		d.Kind, d.Image = store.ScheduledDeployKindImage, req.Image
	default:
		if _, err := rt.gitSources.GetGitSource(r.Context(), name); errors.Is(err, store.ErrGitSourceNotFound) {
			writeError(w, http.StatusBadRequest, "this app has no connected git source; connect one via PUT /api/v1/apps/{name}/git-source before scheduling a ref deploy")
			return
		} else if err != nil {
			rt.internalError(w, "api: create scheduled deploy: load git source failed", err, slog.String("name", name))
			return
		}
		d.Kind, d.Ref = store.ScheduledDeployKindGit, req.Ref
	}
	runAt, err := parseAppScheduleRunAt(req.RunAt, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	d.RunAt = runAt
	if t, id, n, ok := rt.currentActor(r); ok {
		d.CreatedByType, d.CreatedBy, d.CreatedByName = t, id, n
	}
	created, err := rt.scheduledDeploys.CreateScheduledDeploy(r.Context(), d)
	if err != nil {
		rt.internalError(w, "api: create scheduled deploy failed", err, slog.String("name", name))
		return
	}
	rt.recordAppEvent(r, store.AppEvent{AppName: name, Kind: appEventScheduledDeploy,
		Title: "Deploy scheduled for " + created.RunAt.Format(time.RFC3339), Detail: scheduledDeployTarget(created)})
	writeJSON(w, http.StatusCreated, toScheduledDeployResource(created))
}

func scheduledDeployTarget(d store.ScheduledDeploy) string {
	if d.Kind == store.ScheduledDeployKindImage {
		return "image " + d.Image
	}
	return "git ref " + d.Ref
}

// handleListScheduledDeploys handles GET /api/v1/apps/{name}/scheduled-deploys?status=&limit=.
func (rt *Router) handleListScheduledDeploys(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	limit := defaultScheduledDeployListLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = min(n, maxScheduledDeployListLimit)
	}
	var statuses []string
	if raw := r.URL.Query().Get("status"); raw != "" {
		statuses = strings.Split(raw, ",")
	}
	rows, err := rt.scheduledDeploys.ListScheduledDeploys(r.Context(), name, statuses, limit)
	if err != nil {
		rt.internalError(w, "api: list scheduled deploys failed", err, slog.String("name", name))
		return
	}
	out := make([]scheduledDeployResource, 0, len(rows))
	for _, d := range rows {
		out = append(out, toScheduledDeployResource(d))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetScheduledDeploy handles GET /api/v1/apps/{name}/scheduled-deploys/{id}.
func (rt *Router) handleGetScheduledDeploy(w http.ResponseWriter, r *http.Request) {
	d, err := rt.scheduledDeploys.GetScheduledDeploy(r.Context(), r.PathValue("name"), r.PathValue("id"))
	if errors.Is(err, store.ErrScheduledDeployNotFound) {
		writeError(w, http.StatusNotFound, "scheduled deploy not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get scheduled deploy failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toScheduledDeployResource(*d))
}

// handleCancelScheduledDeploy handles DELETE /api/v1/apps/{name}/scheduled-deploys/{id}.
// Only a pending one can be canceled; anything else is 409.
func (rt *Router) handleCancelScheduledDeploy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	d, err := rt.scheduledDeploys.CancelScheduledDeploy(r.Context(), name, r.PathValue("id"), time.Now())
	switch {
	case errors.Is(err, store.ErrScheduledDeployNotFound):
		writeError(w, http.StatusNotFound, "scheduled deploy not found")
		return
	case errors.Is(err, store.ErrScheduledDeployNotPending):
		writeError(w, http.StatusConflict, "only a pending scheduled deploy can be canceled")
		return
	case err != nil:
		rt.internalError(w, "api: cancel scheduled deploy failed", err, slog.String("name", name))
		return
	}
	rt.recordAppEvent(r, store.AppEvent{AppName: name, Kind: appEventScheduledDeploy,
		Title: "Scheduled deploy canceled", Detail: scheduledDeployTarget(*d)})
	writeJSON(w, http.StatusOK, toScheduledDeployResource(*d))
}

// ExecuteScheduledDeploy runs a due scheduled deploy through the same gates a
// manual deploy uses (policy, signature, running-deploy guard, protected
// environment approval). The freeze gate ran in the scanner before this.
func (rt *Router) ExecuteScheduledDeploy(ctx context.Context, d store.ScheduledDeploy) (scheduledeploy.Outcome, error) {
	existing, err := rt.apps.GetDesiredService(ctx, d.ServiceName)
	if errors.Is(err, store.ErrServiceNotFound) {
		return scheduledeploy.Outcome{}, errors.New("app no longer exists")
	}
	if err != nil {
		return scheduledeploy.Outcome{}, fmt.Errorf("load app: %w", err)
	}
	if rt.hasRunningDeployAttempt(ctx, d.ServiceName) {
		return scheduledeploy.Outcome{}, errors.New("a deploy for this app is already running")
	}
	env, protected, err := rt.scheduledDeployEnvironment(ctx, *existing)
	if err != nil {
		return scheduledeploy.Outcome{}, err
	}
	if d.Kind == store.ScheduledDeployKindGit {
		if protected {
			return scheduledeploy.Outcome{}, errors.New("protected environments need an image deploy so it can go through approval")
		}
		msg, err := rt.triggerGitDeploy(ctx, d.ServiceName, d.Ref, store.DeployAttemptSourceSchedule)
		return scheduledeploy.Outcome{Message: msg}, err
	}

	candidate := *existing
	candidate.Image = d.Image
	if set, perr := rt.effectivePolicy(ctx, existing.ProjectID, existing.EnvironmentID); perr != nil {
		return scheduledeploy.Outcome{}, fmt.Errorf("load deploy policy: %w", perr)
	} else if blocking := policy.Blocking(policy.Evaluate(set, deployPolicySubject(candidate))); len(blocking) > 0 {
		return scheduledeploy.Outcome{}, fmt.Errorf("blocked by deploy policy: %s", blocking[0].Message)
	}
	if status, msg := rt.enforceImageSignature(ctx, d.Image); status != 0 {
		return scheduledeploy.Outcome{}, errors.New(msg)
	}
	if protected {
		id, err := rt.requestScheduledDeployApproval(ctx, env, d)
		return scheduledeploy.Outcome{ApprovalID: id}, err
	}
	if _, err := rt.executeConfirmedDeploy(ctx, *existing, d.Image, confirmedDeployOptions{source: store.DeployAttemptSourceSchedule, reason: "Scheduled deploy " + d.ID}); err != nil {
		return scheduledeploy.Outcome{}, err
	}
	return scheduledeploy.Outcome{Message: "deployed " + d.Image}, nil
}

func (rt *Router) scheduledDeployEnvironment(ctx context.Context, svc store.DesiredService) (store.Environment, bool, error) {
	if svc.EnvironmentID == "" {
		return store.Environment{}, false, nil
	}
	env, err := rt.environments.GetEnvironment(ctx, svc.EnvironmentID)
	if errors.Is(err, store.ErrEnvironmentNotFound) {
		return store.Environment{}, false, nil
	}
	if err != nil {
		return store.Environment{}, false, fmt.Errorf("load environment: %w", err)
	}
	return env, env.Protected, nil
}

// requestScheduledDeployApproval files a pending approval attributed to whoever scheduled the deploy.
func (rt *Router) requestScheduledDeployApproval(ctx context.Context, env store.Environment, d store.ScheduledDeploy) (string, error) {
	id, err := store.NewDeployApprovalID()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	a := store.DeployApproval{
		ID: id, ServiceName: d.ServiceName, EnvironmentID: env.ID, Action: store.DeployApprovalActionDeploy, Image: d.Image,
		Status:          store.DeployApprovalStatusPending,
		RequestedByType: d.CreatedByType, RequestedBy: d.CreatedBy, RequestedByName: d.CreatedByName,
		CreatedAt: store.FormatAuditTime(now), ExpiresAt: store.FormatAuditTime(now.Add(rt.effectiveDeployApprovalTTL())),
	}
	if err := rt.deployApprovals.SaveDeployApproval(ctx, a); err != nil {
		return "", fmt.Errorf("save deploy approval: %w", err)
	}
	rt.notifyApprovalRequested(ctx, a)
	return id, nil
}

// NotifyScheduledDeploy records a timeline event and sends a deploy-outcome
// notification when a scheduled deploy was missed, skipped or failed.
func (rt *Router) NotifyScheduledDeploy(ctx context.Context, d store.ScheduledDeploy, status, reason string) {
	if s := rt.appEvents(); s != nil {
		e := store.AppEvent{AppName: d.ServiceName, Kind: appEventScheduledDeploy, Actor: appEventActorSystem,
			Title: "Scheduled deploy " + strings.ReplaceAll(status, "_", " "), Detail: reason}
		if err := s.AddAppEvent(ctx, e); err != nil {
			rt.logger.Error("api: record scheduled deploy event failed", slog.String("error", err.Error()), slog.String("name", d.ServiceName))
		}
	}
	if rt.deployNotifier != nil {
		rt.deployNotifier.Dispatch(ctx, resourceIDForApp(d.ServiceName), alerting.DeployOutcome{
			AppName: d.ServiceName, Image: d.Image, Succeeded: false, Error: "scheduled deploy " + status + ": " + reason,
		})
	}
}
