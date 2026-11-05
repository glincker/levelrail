package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // registers the JPEG decoder for image.Decode
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Audit actions for organization profile changes. Details carry field names and
// slugs only, never description text or image data.
const (
	AuditActionOrgUpdated       = "org.updated"
	AuditActionOrgAvatarUpdated = "org.avatar.updated"
	AuditActionOrgAvatarRemoved = "org.avatar.removed"
	AuditActionOrgSlugChanged   = "org.slug.changed"
)

const (
	orgDescriptionMaxRunes = 500
	orgNameMaxRunes        = 100
	orgSummaryActivity     = 20
	orgActivityPageMax     = 100
	orgFailingAppsMax      = 100
	// A re-encoded PNG can be larger than a compressed JPEG upload; this bounds the growth.
	orgAvatarStoredFactor = 8
	orgAvatarSniffBytes   = 512
)

// orgProfileStore is the store surface of the organization profile and home handlers.
type orgProfileStore interface {
	UpdateOrganizationProfile(ctx context.Context, id string, p store.OrgProfilePatch) (store.Organization, error)
	SetOrganizationAvatar(ctx context.Context, orgID string, a store.OrgAvatar) error
	GetOrganizationAvatar(ctx context.Context, orgID string) (store.OrgAvatar, error)
	DeleteOrganizationAvatar(ctx context.Context, orgID string) error
	OrganizationCounts(ctx context.Context, orgID string, now time.Time) (store.OrgCounts, error)
	OrganizationFailingApps(ctx context.Context, orgID string, limit int) ([]string, error)
	ListTeamEventsPage(ctx context.Context, teamID, curCreated, curID string, limit int) ([]store.TeamEvent, error)
}

func (rt *Router) orgProfile() orgProfileStore {
	if s, ok := rt.organizations.(orgProfileStore); ok {
		return s
	}
	return nil
}

func orgAvatarURL(o store.Organization) string {
	if o.AvatarSHA == "" {
		return ""
	}
	return "/api/v1/organizations/" + o.ID + "/avatar?v=" + o.AvatarSHA[:min(12, len(o.AvatarSHA))]
}

type patchOrganizationRequest struct {
	Name        *string `json:"name"`
	Slug        *string `json:"slug"`
	Description *string `json:"description"`
}

// handlePatchOrganization handles PATCH /api/v1/organizations/{id} and
// PATCH /api/v1/teams/{id}: name, slug and description, org admins only.
func (rt *Router) handlePatchOrganization(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	db := rt.orgProfile()
	if db == nil {
		rt.internalError(w, "api: patch organization", errors.New("store lacks organization profiles"))
		return
	}
	var req patchOrganizationRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if req.Name == nil && req.Slug == nil && req.Description == nil {
		writeError(w, http.StatusBadRequest, "nothing to change; send name, slug or description")
		return
	}
	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if n == "" || utf8.RuneCountInString(n) > orgNameMaxRunes {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("name is required and at most %d characters", orgNameMaxRunes))
			return
		}
		req.Name = &n
	}
	if req.Description != nil {
		d := strings.TrimSpace(*req.Description)
		if utf8.RuneCountInString(d) > orgDescriptionMaxRunes {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("description is at most %d characters", orgDescriptionMaxRunes))
			return
		}
		req.Description = &d
	}
	caller, err := rt.resolveTeamCaller(r, id)
	if err != nil {
		rt.internalError(w, "api: patch organization: caller", err)
		return
	}
	if !caller.canManage(id) {
		writeError(w, http.StatusForbidden, manageDeniedMessage(id))
		return
	}
	before, err := rt.organizations.GetOrganization(r.Context(), id)
	if errors.Is(err, store.ErrOrganizationNotFound) {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: patch organization: load", err, slog.String("id", id))
		return
	}
	var slug *string
	if req.Slug != nil {
		s := strings.ToLower(strings.TrimSpace(*req.Slug))
		slug = &s
	}
	after, err := db.UpdateOrganizationProfile(r.Context(), id, store.OrgProfilePatch{Name: req.Name, Slug: slug, Description: req.Description})
	switch {
	case errors.Is(err, store.ErrOrgSlugTaken):
		writeError(w, http.StatusConflict, "that slug is already in use")
		return
	case errors.Is(err, store.ErrInvalidOrgSlug):
		writeError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), "store: invalid organization slug: "))
		return
	case err != nil:
		rt.internalError(w, "api: patch organization", err, slog.String("id", id))
		return
	}
	fields := changedOrgFields(before, after)
	if len(fields) > 0 {
		rt.auditOrgAction(r, AuditActionOrgUpdated, id, map[string]any{"org_id": id, "fields": fields})
		rt.recordTeamEvent(r, id, caller.userID, AuditActionOrgUpdated, strings.Join(fields, ","), "")
	}
	if before.Slug != after.Slug {
		rt.auditOrgAction(r, AuditActionOrgSlugChanged, id, map[string]any{"org_id": id, "from": before.Slug, "to": after.Slug})
		rt.recordTeamEvent(r, id, caller.userID, AuditActionOrgSlugChanged, after.Slug, "")
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/teams/") {
		rt.handleGetTeam(w, r)
		return
	}
	writeJSON(w, http.StatusOK, toOrganizationResource(after))
}

func changedOrgFields(before, after store.Organization) []string {
	var out []string
	if before.Name != after.Name {
		out = append(out, "name")
	}
	if before.Slug != after.Slug {
		out = append(out, "slug")
	}
	if before.Description != after.Description {
		out = append(out, "description")
	}
	return out
}

// auditOrgAction writes one named audit row for a profile change; best effort.
func (rt *Router) auditOrgAction(r *http.Request, action, orgID string, detail map[string]any) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: org audit id failed", slog.String("action", action))
		return
	}
	a := rt.accessActor(r)
	raw, _ := json.Marshal(detail)
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name, Ability: AbilityWrite,
		Method: r.Method, Path: r.URL.Path, StatusCode: http.StatusOK, RemoteAddr: clientIP(r),
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action: action, Detail: string(raw),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: org audit save failed", slog.String("action", action), slog.String("org_id", orgID), slog.String("error", err.Error()))
		return
	}
	rt.emitAuditEvent(entry)
}

// handlePutOrganizationAvatar handles PUT /api/v1/organizations/{id}/avatar with the raw
// image as the body. The declared content type is ignored: the bytes are sniffed,
// checked from the header, decoded and re-encoded as PNG so no metadata survives.
func (rt *Router) handlePutOrganizationAvatar(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	db := rt.orgProfile()
	if db == nil {
		rt.internalError(w, "api: put avatar", errors.New("store lacks organization profiles"))
		return
	}
	caller, err := rt.resolveTeamCaller(r, id)
	if err != nil {
		rt.internalError(w, "api: put avatar: caller", err)
		return
	}
	if !caller.canManage(id) {
		writeError(w, http.StatusForbidden, manageDeniedMessage(id))
		return
	}
	if _, err := rt.organizations.GetOrganization(r.Context(), id); errors.Is(err, store.ErrOrganizationNotFound) {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: put avatar: load", err, slog.String("id", id))
		return
	}
	maxBytes := int64(rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyOrgsAvatarMaxBytes))
	maxDim := rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyOrgsAvatarMaxDimension)
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("the image is larger than %d bytes", maxBytes))
		return
	}
	encoded, status, msg := reencodeAvatar(raw, maxDim, maxBytes*orgAvatarStoredFactor)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	sum := sha256.Sum256(encoded)
	if err := db.SetOrganizationAvatar(r.Context(), id, store.OrgAvatar{Mime: "image/png", Bytes: encoded, SHA: hex.EncodeToString(sum[:])}); err != nil {
		rt.internalError(w, "api: put avatar", err, slog.String("id", id))
		return
	}
	rt.auditOrgAction(r, AuditActionOrgAvatarUpdated, id, map[string]any{"org_id": id, "bytes": len(encoded)})
	rt.recordTeamEvent(r, id, caller.userID, AuditActionOrgAvatarUpdated, "", "")
	o, err := rt.organizations.GetOrganization(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: put avatar: reload", err, slog.String("id", id))
		return
	}
	writeJSON(w, http.StatusOK, toOrganizationResource(o))
}

// reencodeAvatar returns the PNG bytes, or a non-zero status and message. The header is
// read with DecodeConfig first so a tiny file that declares a huge size is never decoded.
func reencodeAvatar(raw []byte, maxDim int, maxStored int64) ([]byte, int, string) {
	if len(raw) == 0 {
		return nil, http.StatusBadRequest, "the image is empty"
	}
	switch http.DetectContentType(raw[:min(orgAvatarSniffBytes, len(raw))]) {
	case "image/png", "image/jpeg":
	default:
		return nil, http.StatusUnsupportedMediaType, "only PNG and JPEG images are accepted"
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, http.StatusBadRequest, "the image could not be read"
	}
	if cfg.Width > maxDim || cfg.Height > maxDim {
		return nil, http.StatusBadRequest, fmt.Sprintf("the image is larger than %d by %d pixels", maxDim, maxDim)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, http.StatusBadRequest, "the image could not be read"
	}
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img); err != nil {
		return nil, http.StatusBadRequest, "the image could not be re-encoded"
	}
	if int64(buf.Len()) > maxStored {
		return nil, http.StatusRequestEntityTooLarge, "the image is too detailed; use a smaller one"
	}
	return buf.Bytes(), 0, ""
}

// handleGetOrganizationAvatar handles GET /api/v1/organizations/{id}/avatar.
func (rt *Router) handleGetOrganizationAvatar(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	db := rt.orgProfile()
	if db == nil {
		rt.internalError(w, "api: get avatar", errors.New("store lacks organization profiles"))
		return
	}
	a, err := db.GetOrganizationAvatar(r.Context(), id)
	if errors.Is(err, store.ErrOrgAvatarNotFound) {
		writeError(w, http.StatusNotFound, "avatar not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get avatar", err, slog.String("id", id))
		return
	}
	etag := `"` + a.SHA + `"`
	h := w.Header()
	h.Set("Content-Type", a.Mime)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cache-Control", "private, no-cache")
	h.Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Length", strconv.Itoa(len(a.Bytes)))
	_, _ = w.Write(a.Bytes)
}

// handleDeleteOrganizationAvatar handles DELETE /api/v1/organizations/{id}/avatar.
func (rt *Router) handleDeleteOrganizationAvatar(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	db := rt.orgProfile()
	if db == nil {
		rt.internalError(w, "api: delete avatar", errors.New("store lacks organization profiles"))
		return
	}
	caller, err := rt.resolveTeamCaller(r, id)
	if err != nil {
		rt.internalError(w, "api: delete avatar: caller", err)
		return
	}
	if !caller.canManage(id) {
		writeError(w, http.StatusForbidden, manageDeniedMessage(id))
		return
	}
	err = db.DeleteOrganizationAvatar(r.Context(), id)
	if errors.Is(err, store.ErrOrgAvatarNotFound) {
		writeError(w, http.StatusNotFound, "avatar not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: delete avatar", err, slog.String("id", id))
		return
	}
	rt.auditOrgAction(r, AuditActionOrgAvatarRemoved, id, map[string]any{"org_id": id})
	rt.recordTeamEvent(r, id, caller.userID, AuditActionOrgAvatarRemoved, "", "")
	w.WriteHeader(http.StatusNoContent)
}

type orgCountsResource struct {
	Projects       int `json:"projects"`
	Apps           int `json:"apps"`
	Databases      int `json:"databases"`
	Environments   int `json:"environments"`
	Members        int `json:"members"`
	PendingInvites int `json:"pending_invites"`
}

type orgFailingAppsResource struct {
	Count int      `json:"count"`
	Names []string `json:"names"`
}

type orgSummaryResource struct {
	Organization organizationResource   `json:"organization"`
	Counts       orgCountsResource      `json:"counts"`
	FailingApps  orgFailingAppsResource `json:"failing_apps"`
	Activity     []teamEventResource    `json:"activity"`
}

type orgActivityResource struct {
	Items      []teamEventResource `json:"items"`
	NextCursor string              `json:"next_cursor,omitempty"`
}

func toTeamEventResource(e store.TeamEvent) teamEventResource {
	return teamEventResource{ID: e.ID, ActorID: e.ActorID, ActorName: e.ActorName, Action: e.Action, Subject: e.Subject, Detail: e.Detail, CreatedAt: e.CreatedAt.UTC().Format(time.RFC3339)}
}

// handleGetOrganizationSummary handles GET /api/v1/organizations/{id}/summary.
func (rt *Router) handleGetOrganizationSummary(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	db := rt.orgProfile()
	if db == nil {
		rt.internalError(w, "api: org summary", errors.New("store lacks organization profiles"))
		return
	}
	o, err := rt.organizations.GetOrganization(r.Context(), id)
	if errors.Is(err, store.ErrOrganizationNotFound) {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: org summary: load", err, slog.String("id", id))
		return
	}
	counts, err := db.OrganizationCounts(r.Context(), id, time.Now())
	if err != nil {
		rt.internalError(w, "api: org summary: counts", err, slog.String("id", id))
		return
	}
	failing, err := db.OrganizationFailingApps(r.Context(), id, orgFailingAppsMax)
	if err != nil {
		rt.internalError(w, "api: org summary: failing apps", err, slog.String("id", id))
		return
	}
	events, err := db.ListTeamEventsPage(r.Context(), id, "", "", orgSummaryActivity)
	if err != nil {
		rt.internalError(w, "api: org summary: activity", err, slog.String("id", id))
		return
	}
	out := orgSummaryResource{
		Organization: toOrganizationResource(o),
		Counts: orgCountsResource{
			Projects: counts.Projects, Apps: counts.Apps, Databases: counts.Databases,
			Environments: counts.Environments, Members: counts.Members, PendingInvites: counts.PendingInvites,
		},
		FailingApps: orgFailingAppsResource{Count: len(failing), Names: append([]string{}, failing...)},
		Activity:    make([]teamEventResource, 0, len(events)),
	}
	for _, e := range events {
		out.Activity = append(out.Activity, toTeamEventResource(e))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleListOrganizationActivity handles GET /api/v1/organizations/{id}/activity?cursor=&limit=.
func (rt *Router) handleListOrganizationActivity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	db := rt.orgProfile()
	if db == nil {
		rt.internalError(w, "api: org activity", errors.New("store lacks organization profiles"))
		return
	}
	if _, err := rt.organizations.GetOrganization(r.Context(), id); errors.Is(err, store.ErrOrganizationNotFound) {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: org activity: load", err, slog.String("id", id))
		return
	}
	curCreated, curID, ok := decodeActivityCursor(r.URL.Query().Get("cursor"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid cursor")
		return
	}
	limit := orgSummaryActivity
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
		limit = min(n, orgActivityPageMax)
	}
	events, err := db.ListTeamEventsPage(r.Context(), id, curCreated, curID, limit+1)
	if err != nil {
		rt.internalError(w, "api: org activity", err, slog.String("id", id))
		return
	}
	out := orgActivityResource{Items: []teamEventResource{}}
	if len(events) > limit {
		events = events[:limit]
		last := events[len(events)-1]
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(last.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + last.ID))
	}
	for _, e := range events {
		out.Items = append(out.Items, toTeamEventResource(e))
	}
	writeJSON(w, http.StatusOK, out)
}

func decodeActivityCursor(c string) (created, id string, ok bool) {
	if c == "" {
		return "", "", true
	}
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return "", "", false
	}
	created, id, found := strings.Cut(string(raw), "|")
	return created, id, found && created != "" && id != ""
}
