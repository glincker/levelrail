package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/compliance"
	"github.com/GLINCKER/levelrail/internal/store"
)

// SetCompliance enables the access review and evidence pack routes.
func (rt *Router) SetCompliance(s *compliance.Service) { rt.compliance = s }

func (rt *Router) complianceOrNotImplemented(w http.ResponseWriter) (*compliance.Service, bool) {
	if rt.compliance == nil {
		writeError(w, http.StatusNotImplemented, "compliance evidence is not configured on this control plane")
		return nil, false
	}
	return rt.compliance, true
}

// ComplianceSecuritySnapshot is the evidence pack's settings source: the
// stored policy, the live posture checks and the effective retention.
func (rt *Router) ComplianceSecuritySnapshot(ctx context.Context) (any, error) {
	policy, err := rt.securityPolicyResource(ctx)
	if err != nil {
		return nil, fmt.Errorf("security policy: %w", err)
	}
	checks, at := rt.posture(ctx, time.Now())
	return map[string]any{
		"policy":                 policy,
		"posture_checks":         checks,
		"posture_generated_at":   at,
		"session_ttl_hours":      rt.sessionLifetime().Hours(),
		"audit_retention_days":   int(rt.effectiveAuditLogRetention(ctx).Hours() / 24),
		"new_device_approval_on": rt.newDeviceApproval,
	}, nil
}

func (rt *Router) sessionLifetime() time.Duration {
	if rt.sessionTTL > 0 {
		return rt.sessionTTL
	}
	return defaultSessionTTL
}

// ComplianceUserRemover removes an account with the same guards as the
// users API: never yourself, never the last user, never the last root.
func (rt *Router) ComplianceUserRemover() compliance.UserRemover { return complianceUserRemover{rt} }

type complianceUserRemover struct{ rt *Router }

func (c complianceUserRemover) RemoveUser(ctx context.Context, userID, actorID string) error {
	rt := c.rt
	if userID == actorID {
		return errors.New("cannot remove your own account")
	}
	n, err := rt.auth.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if n <= 1 {
		return errors.New("cannot remove the last remaining user")
	}
	if target, gerr := rt.auth.GetUserByID(ctx, userID); gerr == nil && slices.Contains(target.Abilities, AbilityRoot) {
		others, cerr := rt.roles.CountRootUsersExcept(ctx, userID)
		if cerr != nil {
			return fmt.Errorf("count root users: %w", cerr)
		}
		if others == 0 {
			return errors.New("cannot remove the last remaining root user")
		}
	}
	if err := rt.auth.DeleteUser(ctx, userID); err != nil {
		return err
	}
	rt.sessions.revokeAll(userID)
	return nil
}

func (rt *Router) complianceActor(r *http.Request) compliance.Actor {
	a := rt.accessActor(r)
	return compliance.Actor{Type: a.kind, ID: a.id, Name: a.name}
}

func (rt *Router) accessReviewAttentionItems(r *http.Request, abilities []string) []attention.Item {
	if rt.compliance == nil || !hasAbility(abilities, AbilityRoot) {
		return nil
	}
	overdue, err := rt.compliance.Overdue(r.Context())
	if err != nil {
		rt.logger.Warn("api: attention feed: list overdue access reviews failed", slog.String("error", err.Error()))
		return nil
	}
	var items []attention.Item
	for _, c := range overdue {
		items = append(items, feedItem(attention.Warning, attention.KindAccessReviewOverdue, c.Name, "due "+c.DueAt,
			map[string]string{"campaign": c.ID, "due_at": c.DueAt}))
	}
	return items
}

type accessReviewResource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	DueAt       string `json:"due_at"`
	Overdue     bool   `json:"overdue"`
	RepeatDays  int    `json:"repeat_days"`
	CreatedBy   string `json:"created_by"`
	CreatedAt   string `json:"created_at"`
	CompletedAt string `json:"completed_at,omitempty"`
	Total       int    `json:"total"`
	Decided     int    `json:"decided"`
	ToRevoke    int    `json:"to_revoke"`
}

type accessReviewItemResource struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	SubjectID    string `json:"subject_id"`
	Label        string `json:"label"`
	Role         string `json:"role"`
	Detail       string `json:"detail"`
	LastActiveAt string `json:"last_active_at"`
	Decision     string `json:"decision"`
	DecidedBy    string `json:"decided_by,omitempty"`
	DecidedAt    string `json:"decided_at,omitempty"`
	AppliedAt    string `json:"applied_at,omitempty"`
	ApplyError   string `json:"apply_error,omitempty"`
}

type accessReviewDetail struct {
	accessReviewResource
	Items []accessReviewItemResource `json:"items"`
}

func campaignResource(c store.AccessReviewCampaign, items []store.AccessReviewItem) accessReviewResource {
	res := accessReviewResource{ID: c.ID, Name: c.Name, Status: c.Status, DueAt: c.DueAt, RepeatDays: c.RepeatDays,
		CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt, CompletedAt: c.CompletedAt, Total: len(items)}
	if due, err := time.Parse(time.RFC3339, c.DueAt); err == nil {
		res.Overdue = c.Status == store.AccessReviewOpen && time.Now().After(due)
	}
	for _, it := range items {
		if it.Decision != "" {
			res.Decided++
		}
		if it.Decision == store.AccessDecisionRevoke {
			res.ToRevoke++
		}
	}
	return res
}

func (rt *Router) writeComplianceError(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, store.ErrComplianceNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, compliance.ErrUndecided), errors.Is(err, compliance.ErrApplyFailed), errors.Is(err, compliance.ErrPackNotReady):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, compliance.ErrInvalidRange):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		rt.internalError(w, "api: "+op+" failed", err)
	}
}

func (rt *Router) handleListAccessReviews(w http.ResponseWriter, r *http.Request) {
	if _, ok := rt.complianceOrNotImplemented(w); !ok {
		return
	}
	camps, err := rt.compliance.Store().ListAccessReviewCampaigns(r.Context())
	if err != nil {
		rt.writeComplianceError(w, "list access reviews", err)
		return
	}
	out := make([]accessReviewResource, 0, len(camps))
	for _, c := range camps {
		items, err := rt.compliance.Store().ListAccessReviewItems(r.Context(), c.ID)
		if err != nil {
			rt.writeComplianceError(w, "list access review items", err)
			return
		}
		out = append(out, campaignResource(c, items))
	}
	writeJSON(w, http.StatusOK, out)
}

type createAccessReviewRequest struct {
	Name       string `json:"name"`
	DueDays    int    `json:"due_days"`
	RepeatDays int    `json:"repeat_days"`
}

func (rt *Router) handleCreateAccessReview(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.complianceOrNotImplemented(w)
	if !ok {
		return
	}
	var req createAccessReviewRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, msgInvalidBody)
		return
	}
	if req.DueDays < 0 || req.DueDays > 365 || req.RepeatDays < 0 || req.RepeatDays > 366 {
		writeError(w, http.StatusBadRequest, "due_days and repeat_days must be from 0 to 365")
		return
	}
	c, err := svc.CreateCampaign(r.Context(), req.Name, req.DueDays, req.RepeatDays, rt.complianceActor(r))
	if err != nil {
		rt.writeComplianceError(w, "create access review", err)
		return
	}
	items, _ := rt.compliance.Store().ListAccessReviewItems(r.Context(), c.ID)
	writeJSON(w, http.StatusCreated, campaignResource(c, items))
}

func (rt *Router) handleGetAccessReview(w http.ResponseWriter, r *http.Request) {
	if _, ok := rt.complianceOrNotImplemented(w); !ok {
		return
	}
	db := rt.compliance.Store()
	c, err := db.GetAccessReviewCampaign(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.writeComplianceError(w, "get access review", err)
		return
	}
	items, err := db.ListAccessReviewItems(r.Context(), c.ID)
	if err != nil {
		rt.writeComplianceError(w, "get access review items", err)
		return
	}
	d := accessReviewDetail{accessReviewResource: campaignResource(c, items), Items: make([]accessReviewItemResource, 0, len(items))}
	for _, it := range items {
		d.Items = append(d.Items, accessReviewItemResource{ID: it.ID, Kind: it.Kind, SubjectID: it.SubjectID, Label: it.Label,
			Role: it.Role, Detail: it.Detail, LastActiveAt: it.LastActiveAt, Decision: it.Decision, DecidedBy: it.DecidedBy,
			DecidedAt: it.DecidedAt, AppliedAt: it.AppliedAt, ApplyError: it.ApplyError})
	}
	writeJSON(w, http.StatusOK, d)
}

type decideRequest struct {
	Decision string `json:"decision"`
}

func (rt *Router) handleDecideAccessReviewItem(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.complianceOrNotImplemented(w)
	if !ok {
		return
	}
	var req decideRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, msgInvalidBody)
		return
	}
	if req.Decision != store.AccessDecisionKeep && req.Decision != store.AccessDecisionRevoke {
		writeError(w, http.StatusBadRequest, "decision must be keep or revoke")
		return
	}
	if err := svc.Decide(r.Context(), r.PathValue("id"), r.PathValue("item"), req.Decision, rt.complianceActor(r)); err != nil {
		rt.writeComplianceError(w, "decide access review item", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type completeResponse struct {
	Revoked  int                        `json:"revoked"`
	Failures []accessReviewItemResource `json:"failures"`
}

func (rt *Router) handleCompleteAccessReview(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.complianceOrNotImplemented(w)
	if !ok {
		return
	}
	res, err := svc.Complete(r.Context(), r.PathValue("id"), rt.complianceActor(r))
	if errors.Is(err, compliance.ErrApplyFailed) {
		out := completeResponse{Revoked: res.Revoked, Failures: []accessReviewItemResource{}}
		for _, f := range res.Failures {
			out.Failures = append(out.Failures, accessReviewItemResource{ID: f.ID, Kind: f.Kind, Label: f.Label, ApplyError: f.ApplyError})
		}
		writeJSON(w, http.StatusConflict, out)
		return
	}
	if err != nil {
		rt.writeComplianceError(w, "complete access review", err)
		return
	}
	writeJSON(w, http.StatusOK, completeResponse{Revoked: res.Revoked, Failures: []accessReviewItemResource{}})
}

type evidencePackResource struct {
	ID         string `json:"id"`
	RangeFrom  string `json:"range_from"`
	RangeTo    string `json:"range_to"`
	Status     string `json:"status"`
	Step       string `json:"step"`
	Progress   int    `json:"progress"`
	SizeBytes  int64  `json:"size_bytes"`
	SHA256     string `json:"sha256,omitempty"`
	Error      string `json:"error,omitempty"`
	CreatedBy  string `json:"created_by"`
	CreatedAt  string `json:"created_at"`
	FinishedAt string `json:"finished_at,omitempty"`
}

func packResource(p store.EvidencePack) evidencePackResource {
	return evidencePackResource{ID: p.ID, RangeFrom: p.RangeFrom, RangeTo: p.RangeTo, Status: p.Status, Step: p.Step,
		Progress: p.Progress, SizeBytes: p.SizeBytes, SHA256: p.SHA256, Error: p.Error, CreatedBy: p.CreatedBy,
		CreatedAt: p.CreatedAt, FinishedAt: p.FinishedAt}
}

type createPackRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func parseRangeBound(v string, endOfDay bool) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return t, err
	}
	if endOfDay {
		t = t.AddDate(0, 0, 1)
	}
	return t, nil
}

func (rt *Router) handleCreateEvidencePack(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.complianceOrNotImplemented(w)
	if !ok {
		return
	}
	var req createPackRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, msgInvalidBody)
		return
	}
	from, ferr := parseRangeBound(req.From, false)
	to, terr := parseRangeBound(req.To, true)
	if ferr != nil || terr != nil {
		writeError(w, http.StatusBadRequest, "from and to must be dates (YYYY-MM-DD) or RFC3339 times")
		return
	}
	p, err := svc.StartPack(r.Context(), from, to, rt.complianceActor(r))
	if err != nil {
		rt.writeComplianceError(w, "create evidence pack", err)
		return
	}
	writeJSON(w, http.StatusAccepted, packResource(p))
}

func (rt *Router) handleListEvidencePacks(w http.ResponseWriter, r *http.Request) {
	if _, ok := rt.complianceOrNotImplemented(w); !ok {
		return
	}
	packs, err := rt.compliance.Store().ListEvidencePacks(r.Context())
	if err != nil {
		rt.writeComplianceError(w, "list evidence packs", err)
		return
	}
	out := make([]evidencePackResource, 0, len(packs))
	for _, p := range packs {
		out = append(out, packResource(p))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleGetEvidencePack(w http.ResponseWriter, r *http.Request) {
	if _, ok := rt.complianceOrNotImplemented(w); !ok {
		return
	}
	p, err := rt.compliance.Store().GetEvidencePack(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.writeComplianceError(w, "get evidence pack", err)
		return
	}
	writeJSON(w, http.StatusOK, packResource(p))
}

func (rt *Router) handleDownloadEvidencePack(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.complianceOrNotImplemented(w)
	if !ok {
		return
	}
	p, err := rt.compliance.Store().GetEvidencePack(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.writeComplianceError(w, "download evidence pack", err)
		return
	}
	if p.Status != store.EvidencePackComplete {
		rt.writeComplianceError(w, "download evidence pack", compliance.ErrPackNotReady)
		return
	}
	f, err := os.Open(svc.PackPath(p))
	if err != nil {
		rt.internalError(w, "api: open evidence pack failed", err)
		return
	}
	defer func() { _ = f.Close() }()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+p.ID+`.zip"`)
	w.Header().Set("X-Pack-SHA256", p.SHA256)
	http.ServeContent(w, r, p.ID+".zip", time.Time{}, f)
}

func (rt *Router) handleVerifyEvidencePack(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.complianceOrNotImplemented(w)
	if !ok {
		return
	}
	rep, err := svc.VerifyPack(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.writeComplianceError(w, "verify evidence pack", err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
