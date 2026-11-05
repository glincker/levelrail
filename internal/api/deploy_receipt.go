package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/deployreceipt"
	"github.com/GLINCKER/levelrail/internal/deploytimeline"
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
	cfg := deploytimeline.ConfigFromEnv()
	tl := rt.buildTimeline(ctx, a)
	in := deployreceipt.Inputs{
		Attempt:   a,
		Verdict:   string(tl.Verdict),
		Requester: deployreceipt.Person{Kind: tl.Trigger.ActorKind, Name: tl.Trigger.Actor},
		Approval:  rt.receiptApproval(ctx, a, cfg.ActorWindow),
		SBOM:      rt.receiptSBOM(ctx, a),
		Now:       time.Now(),
	}
	if rt.deployGate != nil {
		events, err := rt.deployGate.ListGateEvents(ctx, a.ID)
		if err != nil {
			rt.logger.Warn("api: deploy receipt: list gate events failed", slog.String("deploy_id", a.ID), slog.String("error", err.Error()))
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

// receiptApproval finds the approved request that released the attempt: the
// replay that held it, else the newest approved request for the same image
// made before the attempt started.
func (rt *Router) receiptApproval(ctx context.Context, a store.DeployAttempt, window time.Duration) *store.DeployApproval {
	if rt.deployApprovals == nil {
		return nil
	}
	list, err := rt.deployApprovals.ListDeployApprovals(ctx, store.DeployApprovalStatusApproved, a.ServiceName)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			rt.logger.Warn("api: deploy receipt: list approvals failed", slog.String("deploy_id", a.ID), slog.String("error", err.Error()))
		}
		return nil
	}
	var best *store.DeployApproval
	var bestAt time.Time
	for i := range list {
		ap := list[i]
		if ap.HeldAttemptID == a.ID {
			return &ap
		}
		if ap.Image != a.Image {
			continue
		}
		decided, perr := time.Parse(time.RFC3339Nano, ap.DecidedAt)
		if perr != nil || decided.After(a.StartedAt.Add(window)) {
			continue
		}
		if best == nil || decided.After(bestAt) {
			best, bestAt = &ap, decided
		}
	}
	return best
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
