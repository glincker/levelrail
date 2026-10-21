package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	auditInboxDelete  = "inbox.delete"
	auditInboxRestore = "inbox.restore"
	auditInboxClear   = "inbox.clear"
)

type inboxDeleteRequest struct {
	IDs     []string `json:"ids"`
	Restore bool     `json:"restore,omitempty"`
}

type inboxClearRequest struct {
	Action           string `json:"action"`
	Scope            string `json:"scope"`
	OlderThanSeconds int64  `json:"older_than_seconds,omitempty"`
	DryRun           bool   `json:"dry_run,omitempty"`
	ConfirmCount     int    `json:"confirm_count,omitempty"`
}

type inboxClearResource struct {
	Count  int  `json:"count"`
	DryRun bool `json:"dry_run"`
}

func (rt *Router) handleInboxDelete(w http.ResponseWriter, r *http.Request) {
	principal, ok := rt.inboxPrincipal(w, r)
	if !ok {
		return
	}
	var req inboxDeleteRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := rt.inbox.DeleteInbox(r.Context(), principal, req.IDs, req.Restore, time.Now()); err != nil {
		if errors.Is(err, alerting.ErrInboxInvalid) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		rt.internalError(w, "api: inbox delete failed", err)
		return
	}
	action := auditInboxDelete
	if req.Restore {
		action = auditInboxRestore
	}
	rt.auditInbox(r, action, map[string]any{"count": len(req.IDs), "ids": req.IDs})
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) handleInboxClear(w http.ResponseWriter, r *http.Request) {
	principal, ok := rt.inboxPrincipal(w, r)
	if !ok {
		return
	}
	var req inboxClearRequest
	if err := decodeJSONBody(r, &req); err != nil || req.OlderThanSeconds < 0 {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	sel := alerting.InboxSelector{Scope: req.Scope, OlderThan: time.Duration(req.OlderThanSeconds) * time.Second, Now: time.Now()}
	if req.Action == alerting.InboxActionDelete && !req.DryRun {
		want, err := rt.inbox.ClearInbox(r.Context(), principal, req.Action, sel, true)
		if err == nil && want != req.ConfirmCount {
			err = fmt.Errorf("%w: confirm_count must equal the %d items this would delete", alerting.ErrInboxInvalid, want)
		}
		if err != nil {
			rt.writeInboxError(w, "api: inbox clear preview failed", err)
			return
		}
	}
	n, err := rt.inbox.ClearInbox(r.Context(), principal, req.Action, sel, req.DryRun)
	if err != nil {
		rt.writeInboxError(w, "api: inbox clear failed", err)
		return
	}
	if !req.DryRun {
		rt.auditInbox(r, auditInboxClear, map[string]any{"action": req.Action, "scope": req.Scope, "older_than_seconds": req.OlderThanSeconds, "count": n})
	}
	writeJSON(w, http.StatusOK, inboxClearResource{Count: n, DryRun: req.DryRun})
}

func (rt *Router) writeInboxError(w http.ResponseWriter, msg string, err error) {
	if errors.Is(err, alerting.ErrInboxInvalid) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rt.internalError(w, msg, err)
}

// auditInbox records destructive inbox actions with their scope and count.
func (rt *Router) auditInbox(r *http.Request, action string, detail map[string]any) {
	if rt.auditLog == nil {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: inbox audit id failed", slog.String("error", err.Error()))
		return
	}
	raw, _ := json.Marshal(detail)
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityWrite, Method: r.Method, Path: r.URL.Path, StatusCode: http.StatusOK,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")), Action: action, Detail: string(raw),
	}
	if err := rt.auditLog.SaveAuditEntry(r.Context(), entry); err != nil {
		rt.logger.Warn("api: inbox audit save failed", slog.String("error", err.Error()), slog.String("action", action))
	}
}
