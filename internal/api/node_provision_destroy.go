package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/provision"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AuditActionNodeProvisionDestroy is the audit action for a user-requested server destroy.
const AuditActionNodeProvisionDestroy = "node.provision.destroy"

// nodeProvisionSettled reports statuses the live refresh must not touch again.
func nodeProvisionSettled(status string) bool {
	switch status {
	case store.NodeProvisionStatusReady, store.NodeProvisionStatusFailed,
		store.NodeProvisionStatusDestroying, store.NodeProvisionStatusDestroyed:
		return true
	}
	return false
}

// handleDestroyNodeProvision handles DELETE /api/v1/node-provisions/{id}.
// It deletes only the server id this control plane recorded at create time,
// never one found by name or listing, so it cannot touch foreign machines.
func (rt *Router) handleDestroyNodeProvision(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := rt.nodeProvisions.GetNodeProvision(r.Context(), id)
	if errors.Is(err, store.ErrNodeProvisionNotFound) {
		writeError(w, http.StatusNotFound, "node provision not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: destroy node provision: load failed", err, slog.String("provision_id", id))
		return
	}
	if p.Status == store.NodeProvisionStatusDestroyed {
		writeJSON(w, http.StatusOK, toNodeProvisionResource(p))
		return
	}

	force := r.URL.Query().Get("force") == "true"
	if p.NodeID != "" && !force {
		if busy, err := rt.provisionedNodeHasWorkloads(r.Context(), p.NodeID); err != nil {
			rt.internalError(w, "api: destroy node provision: workload check failed", err, slog.String("provision_id", id))
			return
		} else if busy {
			writeError(w, http.StatusConflict, "the node still runs workloads, drain it or pass force=true")
			return
		}
	}

	if p.ProviderServerID != "" {
		provisioner, ok, status, msg := rt.resolveNodeProvisioner(r.Context(), p.Provider)
		if !ok {
			writeError(w, status, msg)
			return
		}
		if err := rt.markProvisionDestroying(r.Context(), p); err != nil {
			rt.internalError(w, "api: destroy node provision: mark destroying failed", err, slog.String("provision_id", id))
			return
		}
		if err := deleteRecordedServer(r.Context(), provisioner, p.ProviderServerID); err != nil {
			// The provider error can echo request detail, so only the id goes to the log.
			rt.logger.Error("api: destroy node provision: provider delete failed", slog.String("provision_id", id), slog.String("provider", p.Provider))
			rt.auditNodeProvisionDestroy(r, p, http.StatusBadGateway)
			writeError(w, http.StatusBadGateway, "the provider could not delete the server, retry the destroy")
			return
		}
	}

	now := time.Now()
	if err := rt.nodeProvisions.UpdateNodeProvisionStatus(r.Context(), id, store.NodeProvisionStatusDestroyed, "", p.IPAddress, p.NodeID, "", now); err != nil {
		rt.internalError(w, "api: destroy node provision: record destroyed failed", err, slog.String("provision_id", id))
		return
	}
	if p.NodeID != "" {
		if err := rt.nodes.DeleteNode(r.Context(), p.NodeID); err != nil {
			rt.logger.Warn("api: destroy node provision: remove node record failed", slog.String("provision_id", id), slog.String("node_id", p.NodeID), slog.String("error", err.Error()))
		}
	}
	p.Status, p.UpdatedAt, p.FailureReason = store.NodeProvisionStatusDestroyed, now, ""
	rt.auditNodeProvisionDestroy(r, p, http.StatusOK)
	writeJSON(w, http.StatusOK, toNodeProvisionResource(p))
}

func (rt *Router) markProvisionDestroying(ctx context.Context, p store.NodeProvision) error {
	if p.Status == store.NodeProvisionStatusDestroying {
		return nil
	}
	return rt.nodeProvisions.UpdateNodeProvisionStatus(ctx, p.ID, store.NodeProvisionStatusDestroying, "", p.IPAddress, p.NodeID, "", time.Now())
}

// deleteRecordedServer treats a provider 404 as success so a retry after a
// half-finished destroy stays idempotent.
func deleteRecordedServer(ctx context.Context, p NodeProvisioner, serverID string) error {
	err := p.DeleteServer(ctx, serverID)
	var perr *provision.ProviderError
	if errors.As(err, &perr) && perr.Status == http.StatusNotFound {
		return nil
	}
	return err
}

func (rt *Router) provisionedNodeHasWorkloads(ctx context.Context, nodeID string) (bool, error) {
	svcs, err := rt.apps.ListDesiredServicesByNode(ctx, nodeID)
	if err != nil {
		return false, fmt.Errorf("list services: %w", err)
	}
	if len(svcs) > 0 {
		return true, nil
	}
	dbs, err := rt.databases.ListDesiredDatabasesByNode(ctx, nodeID)
	if err != nil {
		return false, fmt.Errorf("list databases: %w", err)
	}
	return len(dbs) > 0, nil
}

func (rt *Router) auditNodeProvisionDestroy(r *http.Request, p store.NodeProvision, status int) {
	if rt.auditLog == nil {
		return
	}
	a := rt.accessActor(r)
	entryID, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	entry := store.AuditEntry{
		ID: entryID, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityRoot, Method: http.MethodDelete, Path: "/api/v1/node-provisions/" + p.ID, StatusCode: status,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action:     AuditActionNodeProvisionDestroy, Detail: fmt.Sprintf(`{"provider":%q,"name":%q}`, p.Provider, p.Name),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: destroy node provision audit save failed", slog.String("error", err.Error()))
	}
}
