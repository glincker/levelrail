package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// deploymentStore is the optional cross-app deployment query surface;
// *store.DB satisfies it.
type deploymentStore interface {
	ListDeployments(ctx context.Context, f store.DeploymentFilter) ([]store.Deployment, string, error)
	ListDeploymentBriefs(ctx context.Context, since time.Time, visibleApps []string) ([]store.DeploymentBrief, error)
}

var errNoDeploymentStore = errors.New("api: deploy attempt store lacks cross-app deployment queries")

type deploymentSteps struct {
	Done        int    `json:"done"`
	Running     int    `json:"running"`
	Failed      int    `json:"failed"`
	FailingStep string `json:"failing_step,omitempty"`
}

// deploymentResource is the wire shape for one row of GET /api/v1/deployments.
type deploymentResource struct {
	ID              string           `json:"id"`
	App             string           `json:"app"`
	Status          string           `json:"status"`
	Trigger         string           `json:"trigger"`
	Environment     string           `json:"environment"`
	Image           string           `json:"image"`
	ImageRef        string           `json:"image_ref"`
	ImageDigest     string           `json:"image_digest"`
	DigestReason    string           `json:"digest_reason"`
	RolloutState    string           `json:"rollout_state"`
	CommitSHA       string           `json:"commit_sha"`
	Branch          string           `json:"branch"`
	CommitMessage   string           `json:"commit_message"`
	Author          string           `json:"author"`
	PRNumber        *int             `json:"pr_number"`
	StartedAt       time.Time        `json:"started_at"`
	FinishedAt      *time.Time       `json:"finished_at"`
	DurationMS      *int64           `json:"duration_ms"`
	Steps           *deploymentSteps `json:"steps"`
	ErrorSummary    *string          `json:"error_summary"`
	ReasonCode      string           `json:"reason_code"`
	Reason          string           `json:"reason"`
	RollbackOf      *string          `json:"rollback_of"`
	RolledBackBy    *string          `json:"rolled_back_by"`
	SupersededBy    *string          `json:"superseded_by"`
	IsLive          bool             `json:"is_live"`
	ApprovalID      *string          `json:"approval_id"`
	PreviewImageURL *string          `json:"preview_image_url"`
	QueuedAt        *time.Time       `json:"queued_at"`
	QueuePosition   *int             `json:"queue_position"`
	WaitReason      *string          `json:"wait_reason"`
	BlockedBy       *string          `json:"blocked_by"`
	CanceledBy      *string          `json:"canceled_by"`
}

type deploymentListResponse struct {
	Items      []deploymentResource `json:"items"`
	NextCursor string               `json:"next_cursor"`
}

func strOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// deploymentImageRef pins the tag to its digest only when the digest is a
// registry-verified one, otherwise the tag is all that can be claimed.
func deploymentImageRef(image, digest, reason string) string {
	if !strings.HasPrefix(digest, "sha256:") {
		return image
	}
	if reason != store.DigestReasonResolved && reason != store.DigestReasonPinned {
		return image
	}
	repo := image
	if i := strings.LastIndex(image, ":"); i > strings.LastIndex(image, "/") {
		repo = image[:i]
	}
	return repo + "@" + digest
}

func deploymentReasonText(code string) string {
	switch code {
	case "":
		return ""
	case store.DeployReasonFrozen:
		return "Held by a freeze window until it ends"
	case store.DeployReasonSuperseded:
		return "A newer deploy replaced this one"
	case store.DeployReasonStale:
		return "A newer commit had already deployed"
	case store.DeployReasonCanceled:
		return "Canceled by an operator before it cut traffic"
	}
	return code
}

func firstLine(s string, limit int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > limit {
		s = s[:limit]
	}
	return s
}

func (rt *Router) toDeploymentResource(d store.Deployment) deploymentResource {
	a := d.Attempt
	res := deploymentResource{
		ID: a.ID, App: a.ServiceName, Status: d.Status, Trigger: d.Trigger, Environment: d.Environment,
		Image: a.Image, ImageRef: deploymentImageRef(a.Image, a.ImageDigest, a.DigestReason),
		ImageDigest: a.ImageDigest, DigestReason: a.DigestReason, RolloutState: a.RolloutState,
		CommitSHA: a.CommitSHA, Branch: a.Branch, CommitMessage: a.CommitMessage, Author: a.Author,
		StartedAt: a.StartedAt, FinishedAt: a.FinishedAt,
		ReasonCode: a.Reason, Reason: deploymentReasonText(a.Reason),
		RollbackOf: strOrNil(d.RollbackOf), RolledBackBy: strOrNil(d.RolledBackBy), SupersededBy: strOrNil(d.SupersededBy),
		IsLive: d.IsLive, QueuedAt: a.QueuedAt, CanceledBy: strOrNil(a.CanceledBy),
	}
	if d.PRNumber > 0 {
		res.PRNumber = &d.PRNumber
	}
	if a.FinishedAt != nil {
		ms := a.FinishedAt.Sub(a.StartedAt).Milliseconds()
		res.DurationMS = &ms
	}
	var failingStep string
	if rt.deployRecorder != nil {
		if sum, ok := rt.deployRecorder.StepSummaryFor(a.ID); ok {
			res.Steps = &deploymentSteps{Done: sum.Done, Running: sum.Running, Failed: sum.Failed, FailingStep: sum.FailingStep}
			failingStep = sum.FailingStep
		}
	}
	if a.Error != "" {
		msg := firstLine(a.Error, 300)
		if failingStep != "" {
			msg = failingStep + ": " + msg
		}
		res.ErrorSummary = &msg
	}
	return res
}

// attachPreviewURLs fills preview_image_url with one preview lookup per
// distinct app on the page; rows are already scoped to readable apps.
func (rt *Router) attachPreviewURLs(ctx context.Context, items []deploymentResource) {
	if rt.preview == nil {
		return
	}
	byApp := make(map[string]map[string]string)
	for i := range items {
		app := items[i].App
		urls, seen := byApp[app]
		if !seen {
			urls = rt.previewImageURLs(ctx, app)
			byApp[app] = urls
		}
		if u, ok := urls[items[i].ID]; ok {
			items[i].PreviewImageURL = &u
		}
	}
}

// visibleAppNames returns the apps the caller can read, or nil when every
// app is visible (a caller with no IAM policies and the base read ability).
func (rt *Router) visibleAppNames(r *http.Request) ([]string, error) {
	canRead, filtered, err := rt.callerAppVisibility(r)
	if err != nil {
		return nil, fmt.Errorf("resolve caller visibility: %w", err)
	}
	if !filtered {
		return nil, nil
	}
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	names := make([]string, 0, len(svcs))
	for _, s := range svcs {
		if canRead(s.Name) {
			names = append(names, s.Name)
		}
	}
	return names, nil
}

// handleListDeployments handles GET /api/v1/deployments: deploy attempts
// across every app the caller can read, newest first, cursor paginated.
func (rt *Router) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	ds, ok := rt.deployAttempts.(deploymentStore)
	if !ok {
		rt.internalError(w, "api: list deployments", errNoDeploymentStore)
		return
	}
	f, err := parseDeploymentQuery(r.URL.Query(), time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if f.VisibleApps, err = rt.visibleAppNames(r); err != nil {
		rt.internalError(w, "api: list deployments: visibility", err)
		return
	}
	rows, next, err := ds.ListDeployments(r.Context(), f)
	if errors.Is(err, store.ErrInvalidDeploymentCursor) {
		writeError(w, http.StatusBadRequest, "cursor is not valid")
		return
	}
	if err != nil {
		rt.internalError(w, "api: list deployments failed", err)
		return
	}
	out := deploymentListResponse{Items: make([]deploymentResource, 0, len(rows)), NextCursor: next}
	waits := rt.deploymentWaits(r.Context(), rows)
	for _, d := range rows {
		res := rt.toDeploymentResource(d)
		applyDeploymentWait(&res, waits[d.Attempt.ID])
		out.Items = append(out.Items, res)
	}
	rt.attachPreviewURLs(r.Context(), out.Items)
	writeJSON(w, http.StatusOK, out)
}

type deploymentDurations struct {
	MedianMS *int64 `json:"median_ms"`
	P95MS    *int64 `json:"p95_ms"`
	Samples  int    `json:"samples"`
}

type deploymentSummaryResponse struct {
	Window         string                `json:"window"`
	Counts         map[string]int        `json:"counts"`
	InProgress     int                   `json:"in_progress"`
	NeedsAttention int                   `json:"needs_attention"`
	FailureRate24h *float64              `json:"failure_rate_24h"`
	Duration       deploymentDurations   `json:"duration"`
	PerDay         []store.DeploymentDay `json:"per_day"`
}

// handleDeploymentsSummary handles GET /api/v1/deployments/summary?window=24h.
func (rt *Router) handleDeploymentsSummary(w http.ResponseWriter, r *http.Request) {
	ds, ok := rt.deployAttempts.(deploymentStore)
	if !ok {
		rt.internalError(w, "api: deployments summary", errNoDeploymentStore)
		return
	}
	window := 24 * time.Hour
	windowLabel := "24h"
	if raw := r.URL.Query().Get("window"); raw != "" {
		d, err := parseWindowDuration(raw)
		if err != nil || d > deploymentsMaxWindow {
			writeError(w, http.StatusBadRequest, "window must be a duration such as 24h or 7d, at most 30d")
			return
		}
		window, windowLabel = d, raw
	}
	visible, err := rt.visibleAppNames(r)
	if err != nil {
		rt.internalError(w, "api: deployments summary: visibility", err)
		return
	}
	now := time.Now().UTC()
	since := now.Add(-window)
	if first := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(store.DeploymentSummaryDays - 1)); first.Before(since) {
		since = first
	}
	briefs, err := ds.ListDeploymentBriefs(r.Context(), since, visible)
	if err != nil {
		rt.internalError(w, "api: deployments summary failed", err)
		return
	}
	s := store.BuildDeploymentSummary(briefs, now, window)
	writeJSON(w, http.StatusOK, deploymentSummaryResponse{
		Window: windowLabel, Counts: s.Counts, InProgress: s.InProgress, NeedsAttention: s.NeedsAttention,
		FailureRate24h: s.FailureRate24h,
		Duration:       deploymentDurations{MedianMS: s.MedianMS, P95MS: s.P95MS, Samples: s.FinishedInRange},
		PerDay:         s.PerDay,
	})
}
