package api

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/announcements"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/version"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

const (
	announcementItemKeyPrefix = "announcement:"
	announcementBodyMaxBytes  = 1 << 10
)

// announcementsHolder lazily builds the Manager the update scheduler polls.
type announcementsHolder struct {
	once sync.Once
	mgr  *announcements.Manager
}

// announcementStore is what the announcement handlers need from the store.
type announcementStore interface {
	announcements.Store
	GetUpdateSettings(ctx context.Context) (store.UpdateSettings, error)
	ListAnnouncementUserState(ctx context.Context, userID string) (map[string]store.AnnouncementUserState, error)
	MarkAnnouncementRead(ctx context.Context, userID, id string, at time.Time) error
	SnoozeAnnouncement(ctx context.Context, userID, id string, until time.Time, entry *store.AuditEntry) error
	DismissAttentionItem(ctx context.Context, userID, itemKey string, at time.Time, entry *store.AuditEntry) (bool, error)
}

func (rt *Router) announcementStore() (announcementStore, bool) {
	st, ok := rt.auditLog.(announcementStore)
	return st, ok
}

// Announcements returns the feed poller for the update scheduler, or nil when
// the store cannot back it.
func (rt *Router) Announcements() *announcements.Manager {
	rt.announcementsState.once.Do(func() {
		st, ok := rt.announcementStore()
		if !ok {
			return
		}
		rt.announcementsState.mgr = &announcements.Manager{
			Store: st, Settings: rt.runtimeSettings, DevMode: devModeEnabled(), Logger: rt.logger,
		}
	})
	return rt.announcementsState.mgr
}

type announcementView struct {
	ID           string               `json:"id"`
	Tier         string               `json:"tier"`
	Title        string               `json:"title"`
	Body         string               `json:"body"`
	AppliesTo    string               `json:"applies_to,omitempty"`
	CTA          string               `json:"cta,omitempty"`
	Links        []announcements.Link `json:"links"`
	IssuedAt     time.Time            `json:"issued_at"`
	ExpiresAt    time.Time            `json:"expires_at"`
	Read         bool                 `json:"read"`
	Dismissed    bool                 `json:"dismissed"`
	SnoozedUntil *time.Time           `json:"snoozed_until,omitempty"`
	Dismissible  bool                 `json:"dismissible"`
}

type announcementStatus struct {
	Mode             string `json:"mode"`
	Sequence         int64  `json:"sequence"`
	FetchedAt        string `json:"fetched_at"`
	VerifiedAt       string `json:"verified_at"`
	LastError        string `json:"last_error"`
	Stale            bool   `json:"stale"`
	TrustConfigured  bool   `json:"trust_configured"`
	AnnouncementsURL string `json:"source_url"`
}

type announcementsResponse struct {
	Items  []announcementView `json:"items"`
	Status announcementStatus `json:"status"`
}

func parseStoreTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// announcementViews returns what the caller may see, with their own read,
// dismissed and snooze marks applied as flags.
func (rt *Router) announcementViews(r *http.Request, abilities []string, now time.Time) ([]announcementView, announcementStatus) {
	status := announcementStatus{Mode: announcements.ModeOff, Stale: false}
	st, ok := rt.announcementStore()
	if !ok {
		return []announcementView{}, status
	}
	ctx := r.Context()
	mode := announcements.Mode(ctx, rt.runtimeSettings)
	status.Mode = mode
	status.AnnouncementsURL = rt.runtimeSettings.String(ctx, runtimesettings.KeyAnnouncementsURL)
	keys, _, _ := announcements.EffectiveKeys(devModeEnabled(), rt.runtimeSettings.List(ctx, runtimesettings.KeyAnnouncementsDevKeys))
	status.TrustConfigured = len(keys) > 0
	if fs, err := st.GetAnnouncementFeedState(ctx); err == nil {
		status.Sequence, status.FetchedAt, status.VerifiedAt, status.LastError = fs.LastSequence, fs.FetchedAt, fs.VerifiedAt, fs.LastError
		status.Stale = fs.ExpiresAt != "" && parseStoreTime(fs.ExpiresAt).Before(now)
	}
	rows, err := st.ListAnnouncements(ctx)
	if err != nil || mode == announcements.ModeOff {
		return []announcementView{}, status
	}
	items := announcements.Visible(rows, announcements.Viewer{
		IsRoot: hasAbilityExactRoot(abilities), CanRead: hasAbility(abilities, AbilityRead),
		Version: version.Version, Channel: rt.announcementChannel(ctx, st), Mode: mode,
		Newsletter: rt.runtimeSettings.Bool(ctx, runtimesettings.KeyAnnouncementsNewsletter), Now: now,
	})
	userID, _ := rt.currentSessionUserID(r)
	var marks map[string]store.AnnouncementUserState
	var dismissed map[string]bool
	if userID != "" {
		marks, _ = st.ListAnnouncementUserState(ctx, userID)
		dismissed = rt.attentionDismissals(r)
	}
	out := make([]announcementView, 0, len(items))
	for _, it := range items {
		v := announcementView{
			ID: it.ID, Tier: it.Tier, Title: it.Title, Body: it.Body, AppliesTo: it.AppliesTo, CTA: it.CTA,
			Links: nonNilLinks(it.Links), IssuedAt: it.IssuedAt, ExpiresAt: it.ExpiresAt,
			Dismissible: it.Tier != announcements.TierCritical,
		}
		m := marks[it.ID]
		v.Read = m.ReadAt != ""
		v.Dismissed = dismissed[announcementItemKeyPrefix+it.ID]
		if until := parseStoreTime(m.SnoozedUntil); until.After(now) {
			v.SnoozedUntil = &until
		}
		out = append(out, v)
	}
	return out, status
}

func nonNilLinks(l []announcements.Link) []announcements.Link {
	if l == nil {
		return []announcements.Link{}
	}
	return l
}

func (rt *Router) announcementChannel(ctx context.Context, st announcementStore) string {
	us, err := st.GetUpdateSettings(ctx)
	if err != nil || !upgrade.ValidChannel(us.Channel) {
		return upgrade.ChannelStable
	}
	return us.Channel
}

// handleListAnnouncements handles GET /api/v1/announcements: verified feed
// items the caller may see (security notices for root only), with status.
func (rt *Router) handleListAnnouncements(w http.ResponseWriter, r *http.Request) {
	abilities, err := rt.callerAbilities(r)
	if err != nil {
		rt.internalError(w, "api: announcements: resolve caller abilities failed", err)
		return
	}
	items, status := rt.announcementViews(r, abilities, time.Now())
	writeJSON(w, http.StatusOK, announcementsResponse{Items: items, Status: status})
}

// handleAnnouncementBanner handles GET /api/v1/announcements/banner: the
// critical notices that are not snoozed, for the top banner.
func (rt *Router) handleAnnouncementBanner(w http.ResponseWriter, r *http.Request) {
	abilities, err := rt.callerAbilities(r)
	if err != nil {
		rt.internalError(w, "api: announcement banner: resolve caller abilities failed", err)
		return
	}
	now := time.Now()
	all, _ := rt.announcementViews(r, abilities, now)
	out := []announcementView{}
	for _, v := range all {
		if v.Tier == announcements.TierCritical && v.SnoozedUntil == nil {
			out = append(out, v)
		}
	}
	writeJSON(w, http.StatusOK, map[string][]announcementView{"items": out})
}

// announcementTarget resolves the path id to an item the caller may see.
func (rt *Router) announcementTarget(w http.ResponseWriter, r *http.Request) (announcementStore, string, announcementView, bool) {
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return nil, "", announcementView{}, false
	}
	st, ok := rt.announcementStore()
	if !ok {
		writeError(w, http.StatusNotImplemented, "announcements are not available")
		return nil, "", announcementView{}, false
	}
	abilities, err := rt.callerAbilities(r)
	if err != nil {
		rt.internalError(w, "api: announcements: resolve caller abilities failed", err)
		return nil, "", announcementView{}, false
	}
	views, _ := rt.announcementViews(r, abilities, time.Now())
	id := r.PathValue("id")
	for _, v := range views {
		if v.ID == id {
			return st, userID, v, true
		}
	}
	writeError(w, http.StatusNotFound, "no such announcement")
	return nil, "", announcementView{}, false
}

// handleAnnouncementRead handles POST /api/v1/announcements/{id}/read.
func (rt *Router) handleAnnouncementRead(w http.ResponseWriter, r *http.Request) {
	st, userID, v, ok := rt.announcementTarget(w, r)
	if !ok {
		return
	}
	if err := st.MarkAnnouncementRead(r.Context(), userID, v.ID, time.Now()); err != nil {
		rt.internalError(w, "api: announcement read failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAnnouncementDismiss handles POST /api/v1/announcements/{id}/dismiss.
// A critical notice cannot be dismissed while it still applies to this build.
func (rt *Router) handleAnnouncementDismiss(w http.ResponseWriter, r *http.Request) {
	st, userID, v, ok := rt.announcementTarget(w, r)
	if !ok {
		return
	}
	if !v.Dismissible {
		writeError(w, http.StatusConflict, "a critical notice cannot be dismissed until the update is installed")
		return
	}
	if _, err := st.DismissAttentionItem(r.Context(), userID, announcementItemKeyPrefix+v.ID, time.Now(), nil); err != nil {
		rt.internalError(w, "api: announcement dismiss failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type announcementSnoozeRequest struct {
	Duration string `json:"duration"`
}

// handleAnnouncementSnooze handles POST /api/v1/announcements/{id}/snooze.
// The snooze is capped by updates.announcements_snooze_max; critical ones are audited.
func (rt *Router) handleAnnouncementSnooze(w http.ResponseWriter, r *http.Request) {
	st, userID, v, ok := rt.announcementTarget(w, r)
	if !ok {
		return
	}
	req, ok := decodeJSON[announcementSnoozeRequest](w, r, announcementBodyMaxBytes)
	if !ok {
		return
	}
	limit := rt.runtimeSettings.Duration(r.Context(), runtimesettings.KeyAnnouncementsSnoozeMax)
	d := limit
	if req.Duration != "" {
		parsed, err := time.ParseDuration(req.Duration)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "duration must be a positive duration such as 4h")
			return
		}
		d = min(parsed, limit)
	}
	now := time.Now()
	var entry *store.AuditEntry
	if v.Tier == announcements.TierCritical {
		e, err := rt.announcementAuditEntry(r, userID, v.ID, store.AuditActionAnnouncementSnoozed, now)
		if err != nil {
			rt.internalError(w, "api: announcement snooze audit failed", err)
			return
		}
		entry = &e
	}
	if err := st.SnoozeAnnouncement(r.Context(), userID, v.ID, now.Add(d), entry); err != nil {
		rt.internalError(w, "api: announcement snooze failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) announcementAuditEntry(r *http.Request, userID, id, action string, now time.Time) (store.AuditEntry, error) {
	entryID, err := store.NewAuditEntryID()
	if err != nil {
		return store.AuditEntry{}, fmt.Errorf("audit id: %w", err)
	}
	return store.AuditEntry{
		ID: entryID, ActorType: auditActorSession, ActorID: userID,
		ActorName: rt.auditActorName(r.Context(), auditActorSession, userID, ""),
		Ability:   AbilityWrite, Method: r.Method, Path: "/api/v1/announcements/" + id,
		StatusCode: http.StatusNoContent, RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(now),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")), Action: action,
	}, nil
}
