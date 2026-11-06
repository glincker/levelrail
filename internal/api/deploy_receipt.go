package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/deployreceipt"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/supplychain"
)

// handleGetDeployReceipt handles GET /api/v1/apps/{name}/deployments/{id}/receipt:
// one unsigned document with the commit, image digest, requester, approver,
// SBOM summary and gate timeline of a deploy. ?format=md returns markdown.
func (rt *Router) handleGetDeployReceipt(w http.ResponseWriter, r *http.Request) {
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format != "" && format != "json" && format != "md" {
		writeError(w, http.StatusBadRequest, "format must be json or md")
		return
	}
	a, ok := rt.timelineAttempt(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	if a.ServiceName != r.PathValue("name") {
		writeError(w, http.StatusNotFound, "deploy not found")
		return
	}
	ctx := r.Context()
	tl := rt.buildTimeline(ctx, a)
	in := deployreceipt.Inputs{
		Attempt:   a,
		Verdict:   string(tl.Verdict),
		Requester: deployreceipt.Person{Kind: tl.Trigger.ActorKind, Name: tl.Trigger.Actor},
		SBOM:      rt.receiptSBOM(ctx, a),
		Now:       time.Now(),
	}
	ap, matchedBy, err := rt.receiptApproval(ctx, a)
	if err != nil {
		rt.logger.Warn("api: deploy receipt: list approvals failed", slog.String("deploy_id", a.ID), slog.String("error", err.Error()))
		in.LookupErrors = append(in.LookupErrors, "approval lookup failed")
	}
	in.Approval, in.ApprovalMatchedBy = ap, matchedBy
	if rt.deployGate != nil {
		events, err := rt.deployGate.ListGateEvents(ctx, a.ID)
		if err != nil {
			rt.logger.Warn("api: deploy receipt: list gate events failed", slog.String("deploy_id", a.ID), slog.String("error", err.Error()))
			in.LookupErrors = append(in.LookupErrors, "gate events lookup failed")
		}
		in.Gate = events
	}
	rec := deployreceipt.Assemble(in)
	w.Header().Set("Cache-Control", "no-store")
	filename := a.ID + ".receipt"
	if format == "md" {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`.md"`)
		_, _ = w.Write([]byte(deployreceipt.Markdown(rec)))
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`.json"`)
	writeJSON(w, http.StatusOK, rec)
}

// receiptApprovalWindow bounds the image-and-time fallback match; the runtime
// setting is the source, the default only covers a router without settings.
func (rt *Router) receiptApprovalWindow(ctx context.Context) time.Duration {
	if rt.runtimeSettings != nil {
		if d := rt.runtimeSettings.Duration(ctx, runtimesettings.KeyDeployReceiptApprovalWindow); d > 0 {
			return d
		}
	}
	return 15 * time.Minute
}

// receiptApproval finds the approval that released the attempt: an exact
// held-attempt link, else an approved deploy, promote or replay decided
// shortly before the attempt started for the same image.
func (rt *Router) receiptApproval(ctx context.Context, a store.DeployAttempt) (*store.DeployApproval, string, error) {
	if rt.deployApprovals == nil {
		return nil, "", nil
	}
	list, err := rt.deployApprovals.ListDeployApprovals(ctx, store.DeployApprovalStatusApproved, a.ServiceName)
	if err != nil {
		return nil, "", fmt.Errorf("list approvals for %s: %w", a.ServiceName, err)
	}
	var best *store.DeployApproval
	var bestAt time.Time
	earliest := a.StartedAt.Add(-rt.receiptApprovalWindow(ctx))
	for i := range list {
		ap := list[i]
		if ap.HeldAttemptID == a.ID {
			return &ap, deployreceipt.MatchedHeldAttempt, nil
		}
		switch ap.Action {
		case store.DeployApprovalActionDeploy, store.DeployApprovalActionPromote, store.DeployApprovalActionReplay:
		default:
			continue
		}
		decided, perr := time.Parse(time.RFC3339Nano, ap.DecidedAt)
		if ap.Image != a.Image || perr != nil || decided.After(a.StartedAt) || decided.Before(earliest) {
			continue
		}
		if best == nil || decided.After(bestAt) {
			best, bestAt = &ap, decided
		}
	}
	if best == nil {
		return nil, "", nil
	}
	return best, deployreceipt.MatchedImageAndTime, nil
}

func (rt *Router) receiptSBOM(ctx context.Context, a store.DeployAttempt) *supplychain.Record {
	if rt.supplyChain == nil {
		return nil
	}
	rec, err := rt.supplyChain.Get(ctx, a.ServiceName, a.ID)
	if err != nil {
		if !errors.Is(err, supplychain.ErrNotFound) && !errors.Is(err, supplychain.ErrNoSBOM) {
			rt.logger.Warn("api: deploy receipt: sbom lookup failed", slog.String("deploy_id", a.ID), slog.String("error", err.Error()))
		}
		return nil
	}
	return &rec
}
