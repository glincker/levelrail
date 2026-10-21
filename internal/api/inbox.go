package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
)

// InboxStore is the surface the notification inbox handlers need from alerting.DB.
type InboxStore interface {
	ListInbox(ctx context.Context, principal string, f alerting.InboxFilter) ([]alerting.InboxItem, error)
	InboxUnreadCount(ctx context.Context, principal string) (int, error)
	MarkInboxRead(ctx context.Context, principal string, ids []string) error
	MarkAllInboxRead(ctx context.Context, principal string, now time.Time) error
	GetInboxState(ctx context.Context, principal string) (alerting.InboxState, error)
	ArchiveInbox(ctx context.Context, principal string, ids []string, archived bool) error
	GetInboxPrefs(ctx context.Context, principal string) (alerting.InboxPrefs, error)
	SaveInboxPrefs(ctx context.Context, p alerting.InboxPrefs, now time.Time) error
}

// WithInbox enables the notification inbox routes.
func WithInbox(s InboxStore) Option {
	return func(rt *Router) { rt.inbox = s }
}

type inboxItemResource struct {
	alertHistoryResource
	Read bool `json:"read"`
}

type inboxPrefsResource struct {
	MinSeverity     string     `json:"min_severity"`
	MutedApps       []string   `json:"muted_apps"`
	DigestEnabled   bool       `json:"digest_enabled"`
	DigestHour      int        `json:"digest_hour"`
	DigestChannelID string     `json:"digest_channel_id"`
	LastDigestAt    *time.Time `json:"last_digest_at,omitempty"`
}

type inboxReadRequest struct {
	IDs []string `json:"ids,omitempty"`
	All bool     `json:"all,omitempty"`
}

type inboxArchiveRequest struct {
	IDs     []string `json:"ids"`
	Restore bool     `json:"restore,omitempty"`
}

type inboxStateResource struct {
	ReadIDs     []string `json:"read_ids"`
	ArchivedIDs []string `json:"archived_ids"`
}

func (rt *Router) handleInboxState(w http.ResponseWriter, r *http.Request) {
	principal, ok := rt.inboxPrincipal(w, r)
	if !ok {
		return
	}
	st, err := rt.inbox.GetInboxState(r.Context(), principal)
	if err != nil {
		rt.internalError(w, "api: inbox state failed", err)
		return
	}
	writeJSON(w, http.StatusOK, inboxStateResource{ReadIDs: st.ReadIDs, ArchivedIDs: st.ArchivedIDs})
}

func (rt *Router) handleInboxArchive(w http.ResponseWriter, r *http.Request) {
	principal, ok := rt.inboxPrincipal(w, r)
	if !ok {
		return
	}
	var req inboxArchiveRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := rt.inbox.ArchiveInbox(r.Context(), principal, req.IDs, !req.Restore); err != nil {
		if errors.Is(err, alerting.ErrInboxInvalid) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		rt.internalError(w, "api: inbox archive failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toInboxPrefsResource(p alerting.InboxPrefs) inboxPrefsResource {
	out := inboxPrefsResource{MinSeverity: p.MinSeverity, MutedApps: p.MutedApps, DigestEnabled: p.DigestEnabled,
		DigestHour: p.DigestHour, DigestChannelID: p.DigestChannelID}
	if out.MutedApps == nil {
		out.MutedApps = []string{}
	}
	if !p.LastDigestAt.IsZero() {
		t := p.LastDigestAt.UTC()
		out.LastDigestAt = &t
	}
	return out
}

// inboxPrincipal keys inbox state to the signed-in user or the bearer token.
func (rt *Router) inboxPrincipal(w http.ResponseWriter, r *http.Request) (string, bool) {
	if rt.inbox == nil {
		writeError(w, http.StatusNotImplemented, "the notification inbox is not configured on this control plane")
		return "", false
	}
	typ, id, _, ok := rt.currentActor(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return "", false
	}
	return typ + ":" + id, true
}

func (rt *Router) handleListInbox(w http.ResponseWriter, r *http.Request) {
	principal, ok := rt.inboxPrincipal(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	f := alerting.InboxFilter{UnreadOnly: q.Get("unread") == "true", Archived: q.Get("archived") == "true"}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		f.Limit = n
	}
	if v := q.Get("before"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "before must be an RFC 3339 timestamp")
			return
		}
		f.Before = &t
	}
	canSee, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: list inbox: visibility", err)
		return
	}
	items, err := rt.inbox.ListInbox(r.Context(), principal, f)
	if err != nil {
		rt.internalError(w, "api: list inbox failed", err)
		return
	}
	out := make([]inboxItemResource, 0, len(items))
	for _, e := range items {
		if e.App != "" && !canSee(e.App) {
			continue
		}
		out = append(out, inboxItemResource{Read: e.Read, alertHistoryResource: alertHistoryResource{
			ID: e.ID, At: e.At.UTC(), RuleID: e.RuleID, RuleName: e.RuleName, RuleKind: e.RuleKind, ResourceID: e.ResourceID,
			App: e.App, Node: e.Node, Severity: e.Severity, Event: e.Event, Outcome: e.Outcome, Detail: e.Detail,
			SilenceID: e.SilenceID, ChannelID: e.ChannelID, Error: e.Error}})
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleInboxUnreadCount(w http.ResponseWriter, r *http.Request) {
	principal, ok := rt.inboxPrincipal(w, r)
	if !ok {
		return
	}
	n, err := rt.inbox.InboxUnreadCount(r.Context(), principal)
	if err != nil {
		rt.internalError(w, "api: inbox unread count failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"unread": n})
}

func (rt *Router) handleInboxMarkRead(w http.ResponseWriter, r *http.Request) {
	principal, ok := rt.inboxPrincipal(w, r)
	if !ok {
		return
	}
	var req inboxReadRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var err error
	if req.All {
		err = rt.inbox.MarkAllInboxRead(r.Context(), principal, time.Now())
	} else {
		err = rt.inbox.MarkInboxRead(r.Context(), principal, req.IDs)
	}
	if errors.Is(err, alerting.ErrInboxInvalid) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: inbox mark read failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) handleGetInboxPrefs(w http.ResponseWriter, r *http.Request) {
	principal, ok := rt.inboxPrincipal(w, r)
	if !ok {
		return
	}
	p, err := rt.inbox.GetInboxPrefs(r.Context(), principal)
	if err != nil {
		rt.internalError(w, "api: get inbox preferences failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toInboxPrefsResource(p))
}

func (rt *Router) handlePutInboxPrefs(w http.ResponseWriter, r *http.Request) {
	principal, ok := rt.inboxPrincipal(w, r)
	if !ok {
		return
	}
	var req inboxPrefsResource
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	muted := make([]string, 0, len(req.MutedApps))
	for _, a := range req.MutedApps {
		if a = strings.TrimSpace(a); a != "" {
			muted = append(muted, a)
		}
	}
	p := alerting.InboxPrefs{Principal: principal, MinSeverity: req.MinSeverity, MutedApps: muted, DigestEnabled: req.DigestEnabled,
		DigestHour: req.DigestHour, DigestChannelID: req.DigestChannelID}
	if err := rt.inbox.SaveInboxPrefs(r.Context(), p, time.Now()); err != nil {
		if errors.Is(err, alerting.ErrInboxInvalid) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		rt.internalError(w, "api: save inbox preferences failed", err)
		return
	}
	saved, err := rt.inbox.GetInboxPrefs(r.Context(), principal)
	if err != nil {
		rt.internalError(w, "api: reload inbox preferences failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toInboxPrefsResource(saved))
}
