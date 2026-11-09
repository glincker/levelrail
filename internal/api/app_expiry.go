package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Runtime settings for app expiry; all apply on the next sweep or request.
const (
	KeyAppExpiryGraceHours    = "deploy.app_expiry_grace_hours"
	KeyAppExpiryNoticeHours   = "deploy.app_expiry_notice_hours"
	KeyAppExpiryMaxPerSweep   = "deploy.app_expiry_max_per_sweep"
	KeyAppExpiryMaxHorizonDay = "deploy.app_expiry_max_horizon_days"
	KeyAppExpiryDeleteEnabled = "deploy.app_expiry_delete_enabled"
	KeyAppExpiryClaimTTL      = "deploy.app_expiry_claim_ttl"
)

const (
	appExpiryClaimTTLDefault = time.Hour
	appExpiryClaimTTLFloor   = 5 * time.Minute
	appExpiryClaimTTLCeiling = 24 * time.Hour
)

type expirySettingBounds struct{ def, lo, hi int }

var appExpiryBounds = map[string]expirySettingBounds{
	KeyAppExpiryGraceHours:    {168, 1, 2160},
	KeyAppExpiryNoticeHours:   {24, 0, 720},
	KeyAppExpiryMaxPerSweep:   {20, 1, 500},
	KeyAppExpiryMaxHorizonDay: {90, 1, 3650},
}

func init() {
	defs := []struct{ key, title, desc string }{
		{KeyAppExpiryGraceHours, "App expiry grace period (hours)", "Hours an expired app stays stopped before the sweep may delete it. Between 1 hour and 90 days. Clearing the expiry in that window cancels the delete."},
		{KeyAppExpiryNoticeHours, "App expiry notice lead (hours)", "Hours before an app's expiry that the notice is written to the audit log. The stop always waits for the notice."},
		{KeyAppExpiryMaxPerSweep, "App expiry actions per sweep", "Most apps one expiry sweep notifies, stops or deletes. The rest wait for the next sweep."},
		{KeyAppExpiryMaxHorizonDay, "App expiry max horizon (days)", "Furthest in the future an app expiry may be set."},
	}
	for _, d := range defs {
		b := appExpiryBounds[d.key]
		runtimesettings.Register(runtimesettings.Definition{
			Key: d.key, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindInt,
			Default: strconv.Itoa(b.def), Apply: runtimesettings.ApplyLive,
			CheckValue: func(canonical string) error {
				if n, err := strconv.Atoi(canonical); err != nil || n < b.lo || n > b.hi {
					return fmt.Errorf("value must be between %d and %d", b.lo, b.hi)
				}
				return nil
			},
			Title: d.title, Description: d.desc,
		})
	}
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyAppExpiryDeleteEnabled, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindBool,
		Default: "false", Apply: runtimesettings.ApplyLive,
		Title:       "App expiry deletes apps",
		Description: "Off by default: expired apps are only noticed and stopped. Turn on to let the sweep delete them after the grace period.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyAppExpiryClaimTTL, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindDuration,
		Default: appExpiryClaimTTLDefault.String(), Apply: runtimesettings.ApplyLive,
		CheckValue: func(canonical string) error {
			if d, err := time.ParseDuration(canonical); err != nil || d < appExpiryClaimTTLFloor || d > appExpiryClaimTTLCeiling {
				return fmt.Errorf("value must be between %s and %s", appExpiryClaimTTLFloor, appExpiryClaimTTLCeiling)
			}
			return nil
		},
		Title:       "App expiry delete claim lifetime",
		Description: "How long a delete claim on an expired app may stay before the sweep treats it as abandoned and releases it. Between 5m and 24h.",
	})
}

func (rt *Router) appExpiryClaimTTL(ctx context.Context) time.Duration {
	d := appExpiryClaimTTLDefault
	if rt.runtimeSettings != nil {
		if v := rt.runtimeSettings.Duration(ctx, KeyAppExpiryClaimTTL); v > 0 {
			d = v
		}
	}
	return min(max(d, appExpiryClaimTTLFloor), appExpiryClaimTTLCeiling)
}

// appExpirySetting reads a live setting and clamps it, so a bad row never widens the bounds.
func (rt *Router) appExpirySetting(ctx context.Context, key string) int {
	b := appExpiryBounds[key]
	n := b.def
	if rt.runtimeSettings != nil {
		n = rt.runtimeSettings.Int(ctx, key)
	}
	return min(max(n, b.lo), b.hi)
}

func (rt *Router) appExpiryDeleteEnabled(ctx context.Context) bool {
	return rt.runtimeSettings != nil && rt.runtimeSettings.Bool(ctx, KeyAppExpiryDeleteEnabled)
}

type appExpiryStore interface {
	GetAppExpiry(ctx context.Context, app string) (*store.AppExpiry, error)
	SetAppExpiry(ctx context.Context, e store.AppExpiry) error
	ClearAppExpiry(ctx context.Context, app string) (bool, error)
	ListAppExpiries(ctx context.Context) ([]store.AppExpiry, error)
	MarkAppExpiry(ctx context.Context, e store.AppExpiry, notifiedAt, stoppedAt, stoppedBy, keptReason string) (bool, error)
	UnstampAppExpiryStop(ctx context.Context, app, stoppedAt string) error
	ClaimAppExpiry(ctx context.Context, e store.AppExpiry, claimedAt string) (bool, error)
	ReleaseAppExpiryClaim(ctx context.Context, app, claimedAt string) error
	RemoveAppExpiryRow(ctx context.Context, e store.AppExpiry) error
}

type serviceIncarnationStore interface {
	ClaimServiceIncarnation(ctx context.Context, name string) (string, bool, error)
}

type appExpiryResource struct {
	App            string           `json:"app"`
	Expiry         *store.AppExpiry `json:"expiry,omitempty"`
	State          string           `json:"state"`
	DeleteAfter    string           `json:"delete_after,omitempty"`
	DeleteEnabled  bool             `json:"delete_enabled"`
	GraceHours     int              `json:"grace_hours"`
	NoticeHours    int              `json:"notice_hours"`
	MaxHorizonDays int              `json:"max_horizon_days"`
}

type setAppExpiryRequest struct {
	ExpiresAt  string `json:"expires_at"`
	DeleteData bool   `json:"delete_data"`
	Confirm    string `json:"confirm"`
}

func (rt *Router) expiryStore(w http.ResponseWriter) (appExpiryStore, bool) {
	st, ok := rt.apps.(appExpiryStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "app expiry is not available")
	}
	return st, ok
}

func (rt *Router) toAppExpiryResource(ctx context.Context, app string, e *store.AppExpiry) appExpiryResource {
	grace := rt.appExpirySetting(ctx, KeyAppExpiryGraceHours)
	out := appExpiryResource{App: app, Expiry: e, State: "none", GraceHours: grace, DeleteEnabled: rt.appExpiryDeleteEnabled(ctx),
		NoticeHours: rt.appExpirySetting(ctx, KeyAppExpiryNoticeHours), MaxHorizonDays: rt.appExpirySetting(ctx, KeyAppExpiryMaxHorizonDay)}
	switch {
	case e == nil:
	case e.KeptReason != "":
		out.State = "kept"
	case e.StoppedAt != "":
		out.State = "stopped"
		if t, err := time.Parse(time.RFC3339, e.StoppedAt); err == nil && out.DeleteEnabled {
			out.DeleteAfter = t.Add(time.Duration(grace) * time.Hour).UTC().Format(time.RFC3339)
		}
	case e.NotifiedAt != "":
		out.State = "noticed"
	default:
		out.State = "scheduled"
	}
	return out
}

// loadExpiryApp writes the 404 itself, so callers only see apps that exist.
func (rt *Router) loadExpiryApp(w http.ResponseWriter, r *http.Request) (*store.DesiredService, appExpiryStore, bool) {
	name := r.PathValue("name")
	st, ok := rt.expiryStore(w)
	if !ok {
		return nil, nil, false
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		rt.writeAppNotFound(r.Context(), w, name)
		return nil, nil, false
	} else if err != nil {
		rt.internalError(w, "api: app expiry: load app", err, slog.String("name", name))
		return nil, nil, false
	}
	return svc, st, true
}

// handleGetAppExpiry handles GET /api/v1/apps/{name}/expiry.
func (rt *Router) handleGetAppExpiry(w http.ResponseWriter, r *http.Request) {
	svc, st, ok := rt.loadExpiryApp(w, r)
	if !ok {
		return
	}
	e, err := st.GetAppExpiry(r.Context(), svc.Name)
	if err != nil {
		rt.internalError(w, "api: get app expiry", err, slog.String("name", svc.Name))
		return
	}
	writeJSON(w, http.StatusOK, rt.toAppExpiryResource(r.Context(), svc.Name, e))
}

// handleSetAppExpiry handles PUT /api/v1/apps/{name}/expiry (write, the app
// delete tier). delete_data also needs write:sensitive, a local node and confirm.
func (rt *Router) handleSetAppExpiry(w http.ResponseWriter, r *http.Request) {
	svc, st, ok := rt.loadExpiryApp(w, r)
	if !ok {
		return
	}
	name := svc.Name
	var req setAppExpiryRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	at, err := time.Parse(time.RFC3339, strings.TrimSpace(req.ExpiresAt))
	if err != nil {
		writeError(w, http.StatusBadRequest, "expires_at must be an RFC 3339 time, for example 2026-12-01T18:00:00Z")
		return
	}
	now := time.Now().UTC()
	horizon := rt.appExpirySetting(r.Context(), KeyAppExpiryMaxHorizonDay)
	if !at.After(now) {
		writeError(w, http.StatusBadRequest, "expires_at must be in the future")
		return
	}
	if at.After(now.Add(time.Duration(horizon) * 24 * time.Hour)) {
		writeError(w, http.StatusBadRequest, "expires_at must be within "+strconv.Itoa(horizon)+" days")
		return
	}
	if req.DeleteData {
		if !rt.callerHasAbility(r, AbilityWriteSensitive) {
			writeError(w, http.StatusForbidden, "delete_data needs the write:sensitive ability")
			return
		}
		if !rt.isLocalNode(svc.NodeID) {
			writeError(w, http.StatusBadRequest, "delete_data only works for apps on the control plane's own node")
			return
		}
		if req.Confirm != name {
			writeError(w, http.StatusBadRequest, "delete_data needs confirm set to the exact app name: volumes and data may be removed after the grace period")
			return
		}
	}
	unlock := lockAppName(name)
	defer unlock()
	if cur, err := rt.apps.GetDesiredService(r.Context(), name); err != nil || cur.Incarnation != svc.Incarnation {
		writeError(w, http.StatusConflict, "the app changed while the expiry was being set; try again")
		return
	}
	if rt.refuseIfProtected(w, r, name) {
		return
	}
	// The sweep never stops or deletes an app in a protected environment, so an expiry there is refused.
	if svc.EnvironmentID != "" {
		if rt.environments == nil {
			writeError(w, http.StatusConflict, "environment protection cannot be checked; expiry is not available for this app")
			return
		}
		env, eerr := rt.environments.GetEnvironment(r.Context(), svc.EnvironmentID)
		if eerr != nil {
			rt.internalError(w, "api: set app expiry: load environment", eerr, slog.String("name", name))
			return
		}
		if env.Protected {
			writeError(w, http.StatusConflict, "app is in a protected environment: expiry is not available for it")
			return
		}
	}
	retained, rerr := rt.retainedDeployServicesStrict(r.Context())
	if rerr != nil {
		rt.internalError(w, "api: set app expiry: list retained deploys", rerr, slog.String("name", name))
		return
	}
	if retained[name] {
		writeError(w, http.StatusConflict, "this is a retained copy of an earlier deploy that the platform manages; it is removed automatically")
		return
	}
	incarnation := svc.Incarnation
	if is, ok := rt.apps.(serviceIncarnationStore); ok && incarnation == "" {
		var updated bool
		if incarnation, updated, err = is.ClaimServiceIncarnation(r.Context(), name); err != nil {
			rt.internalError(w, "api: set app expiry: incarnation", err, slog.String("name", name))
			return
		}
		if !updated {
			writeError(w, http.StatusConflict, "the app changed while the expiry was being set; try again")
			return
		}
	}
	if incarnation == "" {
		writeError(w, http.StatusConflict, "this app has no stable identity yet; redeploy it, then set the expiry")
		return
	}
	setterType, setterID, _, perr := rt.callerPrincipal(r)
	if perr != nil || setterID == "" {
		writeError(w, http.StatusForbidden, "the caller could not be identified")
		return
	}
	e := store.AppExpiry{AppName: name, ExpiresAt: at.UTC().Format(time.RFC3339), DeleteData: req.DeleteData,
		SetBy: rt.actorLabel(r), SetAt: now.Format(time.RFC3339Nano), Incarnation: incarnation, SetterType: setterType, SetterID: setterID}
	if err := st.SetAppExpiry(r.Context(), e); errors.Is(err, store.ErrAppExpiryClaimed) {
		writeError(w, http.StatusConflict, "a scheduled delete of this app is running; try again when it finishes")
		return
	} else if err != nil {
		rt.internalError(w, "api: set app expiry", err, slog.String("name", name))
		return
	}
	rt.auditAppExpirySet(r, e)
	saved, err := st.GetAppExpiry(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: set app expiry: reload", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, rt.toAppExpiryResource(r.Context(), name, saved))
}

// handleClearAppExpiry handles DELETE /api/v1/apps/{name}/expiry. It takes the
// name lock, so it either lands before a delete claim or after the app is gone.
func (rt *Router) handleClearAppExpiry(w http.ResponseWriter, r *http.Request) {
	unlock := lockAppName(r.PathValue("name"))
	defer unlock()
	svc, st, ok := rt.loadExpiryApp(w, r)
	if !ok {
		return
	}
	if _, err := st.ClearAppExpiry(r.Context(), svc.Name); err != nil {
		rt.internalError(w, "api: clear app expiry", err, slog.String("name", svc.Name))
		return
	}
	writeJSON(w, http.StatusOK, rt.toAppExpiryResource(r.Context(), svc.Name, nil))
}

func appExpiryAuditDetail(app, reason string) string {
	d := map[string]string{"app": app}
	if reason != "" {
		d["reason"] = reason
	}
	b, _ := json.Marshal(d)
	return string(b)
}

func (rt *Router) saveAppExpiryAudit(ctx context.Context, entry store.AuditEntry) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: app expiry audit id failed", slog.String("error", err.Error()))
		return
	}
	entry.ID, entry.CreatedAt = id, store.FormatAuditTime(time.Now())
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(ctx), entry); err != nil {
		rt.logger.Warn("api: save app expiry audit failed", slog.String("error", err.Error()))
	}
}

func (rt *Router) recordAppExpiryAudit(ctx context.Context, action, app, reason string) {
	rt.saveAppExpiryAudit(ctx, store.AuditEntry{
		ActorType: auditActorSystem, ActorID: "app-expiry", ActorName: "app expiry",
		Ability: AbilityRoot, Method: http.MethodPost, Path: "/api/v1/apps/" + app + "/expiry", StatusCode: http.StatusOK,
		ClientKind: "system", Action: action, Detail: appExpiryAuditDetail(app, reason),
	})
}

func (rt *Router) auditAppExpirySet(r *http.Request, e store.AppExpiry) {
	a := rt.accessActor(r)
	b, _ := json.Marshal(map[string]any{"app": e.AppName, "expires_at": e.ExpiresAt, "delete_data": e.DeleteData})
	rt.saveAppExpiryAudit(r.Context(), store.AuditEntry{
		ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityWrite, Method: http.MethodPut, Path: "/api/v1/apps/" + e.AppName + "/expiry", StatusCode: http.StatusOK,
		RemoteAddr: clientIP(r), ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action: "app.expiry.set", Detail: string(b),
	})
}

// retainedDeployServicesStrict is retainedDeployServices that reports a
// failed lookup, so a destructive caller can wait instead.
func (rt *Router) retainedDeployServicesStrict(ctx context.Context) (map[string]bool, error) {
	l, ok := rt.deployAttempts.(deployURLLister)
	if !ok {
		return map[string]bool{}, nil
	}
	rows, err := l.ListDeployURLs(ctx, "")
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows))
	for _, r := range rows {
		out[r.ServiceName] = true
	}
	return out, nil
}

// appExpiryStopKeepReason says why the sweep must not even stop svc. A lookup
// error is returned instead, so the row stays unchanged and the next sweep retries.
func (rt *Router) appExpiryStopKeepReason(ctx context.Context, svc store.DesiredService, _ store.AppExpiry) (string, error) {
	if st, ok := rt.apps.(appProtectionStore); ok {
		p, err := st.GetAppProtection(ctx, svc.Name)
		if err != nil {
			return "", err
		}
		if p != nil {
			return "app is protected", nil
		}
	}
	retained, err := rt.retainedDeployServicesStrict(ctx)
	if err != nil {
		return "", err
	}
	if retained[svc.Name] {
		return "retained deploy copy", nil
	}
	if svc.EnvironmentID != "" {
		if rt.environments == nil {
			return "environment protection unknown", nil
		}
		env, err := rt.environments.GetEnvironment(ctx, svc.EnvironmentID)
		if err != nil {
			return "", err
		}
		if env.Protected {
			return "app is in a protected environment", nil
		}
	}
	return "", nil
}

// appExpiryKeepReason says why the sweep must not delete svc, or "" when it may.
// Data, bind mounts, databases and dependents keep unless delete_data.
func (rt *Router) appExpiryKeepReason(ctx context.Context, svc store.DesiredService, e store.AppExpiry) (string, error) {
	if !svc.Suspended {
		return store.AppExpiryKeptStarted, nil
	}
	if reason, err := rt.appExpiryStopKeepReason(ctx, svc, e); reason != "" || err != nil {
		return reason, err
	}
	if e.DeleteData {
		return rt.appExpirySetterReason(ctx, e)
	}
	for _, v := range svc.Volumes {
		if v.Name != deploy.NextImageCacheVolume(svc.Name) {
			return "app has persistent volumes", nil
		}
	}
	if len(svc.BindMounts) > 0 {
		return "app has bind mounts", nil
	}
	if len(svc.DatabaseEnv) > 0 || svc.DatabaseAttachment != nil {
		return "app has linked databases", nil
	}
	all, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return "", err
	}
	for _, other := range all {
		for _, dep := range other.DependsOn {
			if dep == svc.Name && other.Name != svc.Name {
				return "other apps depend on it", nil
			}
		}
	}
	return "", nil
}

const appExpirySetterGone = "setter no longer authorized"

// appExpirySetterReason re-checks that whoever asked for delete_data still
// holds write:sensitive; a removed user or revoked token keeps the app.
func (rt *Router) appExpirySetterReason(ctx context.Context, e store.AppExpiry) (string, error) {
	switch e.SetterType {
	case store.PrincipalTypeUser:
		if rt.auth == nil {
			return appExpirySetterGone, nil
		}
		user, err := rt.auth.GetUserByID(ctx, e.SetterID)
		if errors.Is(err, store.ErrUserNotFound) {
			return appExpirySetterGone, nil
		}
		if err != nil {
			return "", err
		}
		if !hasAbility(user.Abilities, AbilityWriteSensitive) || rt.accountDisabled(ctx, e.SetterID) {
			return appExpirySetterGone, nil
		}
		return "", nil
	case store.PrincipalTypeToken:
		if rt.tokens == nil {
			return appExpirySetterGone, nil
		}
		rec, err := rt.tokens.GetAPITokenByID(ctx, e.SetterID)
		if errors.Is(err, store.ErrAPITokenNotFound) {
			return appExpirySetterGone, nil
		}
		if err != nil {
			return "", err
		}
		if rec.RevokedAt != nil || (rec.ExpiresAt != nil && time.Now().After(*rec.ExpiresAt)) || !hasAbility(rec.Abilities, AbilityWriteSensitive) {
			return appExpirySetterGone, nil
		}
		gone, err := rt.tokenOwnerGone(ctx, rec)
		if err != nil {
			return "", err
		}
		if gone {
			return appExpirySetterGone, nil
		}
		return "", nil
	}
	return appExpirySetterGone, nil
}

func (rt *Router) appDeployRunning(ctx context.Context, name string) (bool, error) {
	if rt.deploySafety == nil {
		return false, nil
	}
	attempts, err := rt.deploySafety.ListRunningDeployAttempts(ctx)
	if err != nil {
		return true, err
	}
	for _, a := range attempts {
		if a.ServiceName == name {
			return true, nil
		}
	}
	return false, nil
}

var errAppExpiryNotClaimed = errors.New("app expiry changed before the delete")

// SweepAppExpiry walks expiries soonest first: notice, then stop at expiry,
// then delete after the grace period. Progress lives in the row, so reruns are no-ops.
func (rt *Router) SweepAppExpiry(ctx context.Context, now time.Time) (acted int, err error) {
	st, ok := rt.apps.(appExpiryStore)
	if !ok {
		return 0, nil
	}
	all, err := st.ListAppExpiries(ctx)
	if err != nil {
		return 0, fmt.Errorf("api: sweep app expiry: %w", err)
	}
	limit := rt.appExpirySetting(ctx, KeyAppExpiryMaxPerSweep)
	lead := time.Duration(rt.appExpirySetting(ctx, KeyAppExpiryNoticeHours)) * time.Hour
	grace := time.Duration(rt.appExpirySetting(ctx, KeyAppExpiryGraceHours)) * time.Hour
	var errs []error
	for _, e := range all {
		if acted >= limit {
			break
		}
		expires, perr := time.Parse(time.RFC3339, e.ExpiresAt)
		if e.ClaimedAt != "" {
			rt.releaseStaleAppExpiryClaim(ctx, st, e, now, &errs)
			continue
		}
		if perr != nil || e.KeptReason != "" || now.Before(expires.Add(-lead)) {
			continue
		}
		did, aerr := rt.advanceAppExpiry(ctx, st, e, now, expires, grace)
		if aerr != nil {
			errs = append(errs, fmt.Errorf("app %q: %w", e.AppName, aerr))
		}
		if did {
			acted++
		}
	}
	return acted, errors.Join(errs...)
}

// releaseStaleAppExpiryClaim drops a claim older than the claim lifetime, left
// by a delete that never finished, so the app is looked at again next sweep.
func (rt *Router) releaseStaleAppExpiryClaim(ctx context.Context, st appExpiryStore, e store.AppExpiry, now time.Time, errs *[]error) {
	claimed, err := time.Parse(time.RFC3339Nano, e.ClaimedAt)
	if err == nil && now.Sub(claimed) < rt.appExpiryClaimTTL(ctx) {
		return
	}
	if err := st.ReleaseAppExpiryClaim(ctx, e.AppName, e.ClaimedAt); err != nil {
		*errs = append(*errs, fmt.Errorf("app %q: %w", e.AppName, err))
		return
	}
	rt.recordAppExpiryAudit(ctx, "app.expiry.claim.released", e.AppName, "stale claim")
}

// advanceAppExpiry moves one app at most one step, so a notice always lands a sweep before the stop.
func (rt *Router) advanceAppExpiry(ctx context.Context, st appExpiryStore, e store.AppExpiry, now, expires time.Time, grace time.Duration) (bool, error) {
	if busy, err := rt.appDeployRunning(ctx, e.AppName); busy || err != nil {
		return false, err
	}
	svc, err := rt.apps.GetDesiredService(ctx, e.AppName)
	if errors.Is(err, store.ErrServiceNotFound) || (err == nil && (e.Incarnation == "" || svc.Incarnation != e.Incarnation)) {
		// Another life of the name never inherits this row.
		return false, st.RemoveAppExpiryRow(ctx, e)
	}
	if err != nil {
		return false, err
	}
	switch {
	case e.NotifiedAt == "":
		if ok, err := st.MarkAppExpiry(ctx, e, now.UTC().Format(time.RFC3339), "", "", ""); err != nil || !ok {
			return false, err
		}
		rt.recordAppExpiryAudit(ctx, "app.expiry.notice", e.AppName, "")
		return true, nil
	case e.StoppedAt == "":
		if now.Before(expires) {
			return false, nil
		}
		return rt.stopExpiredApp(ctx, st, e, now)
	}
	stopped, err := time.Parse(time.RFC3339, e.StoppedAt)
	if err != nil || now.Before(stopped.Add(grace)) || !rt.appExpiryDeleteEnabled(ctx) {
		return false, nil
	}
	return rt.deleteExpiredApp(ctx, st, e)
}

func (rt *Router) keepAppExpiry(ctx context.Context, st appExpiryStore, e store.AppExpiry, reason string) (bool, error) {
	if ok, err := st.MarkAppExpiry(ctx, e, e.NotifiedAt, e.StoppedAt, e.StoppedBy, reason); err != nil || !ok {
		return false, err
	}
	rt.recordAppExpiryAudit(ctx, "app.expiry.kept", e.AppName, reason)
	return true, nil
}

// stopExpiredApp stamps the stop first and suspends second, under the name
// lock, and rolls the stamp back if the suspend fails.
func (rt *Router) stopExpiredApp(ctx context.Context, st appExpiryStore, e store.AppExpiry, now time.Time) (bool, error) {
	unlock := lockAppName(e.AppName)
	defer unlock()
	svc, err := rt.apps.GetDesiredService(ctx, e.AppName)
	if err != nil {
		return false, err
	}
	if svc.Incarnation != e.Incarnation {
		return false, nil
	}
	reason, err := rt.appExpiryStopKeepReason(ctx, *svc, e)
	if err != nil {
		return false, err
	}
	if reason != "" {
		return rt.keepAppExpiry(ctx, st, e, reason)
	}
	stoppedBy := "already_stopped"
	if !svc.Suspended {
		if rt.offlineNodeFor(ctx, *svc) != nil {
			return false, nil // retried next sweep, once the node is back
		}
		req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, "/", nil)
		if rerr != nil {
			return false, rerr
		}
		if derr := rt.checkDisruption(req, *svc, disruptionStop); derr != nil {
			return false, nil //nolint:nilerr // a disruption budget holds the stop until it allows one
		}
		stoppedBy = "sweep"
	}
	stamp := now.UTC().Format(time.RFC3339)
	if ok, err := st.MarkAppExpiry(ctx, e, e.NotifiedAt, stamp, stoppedBy, ""); err != nil || !ok {
		return false, err
	}
	if stoppedBy == "sweep" {
		if err := rt.apps.UpdateServiceSuspended(ctx, e.AppName, true); err != nil {
			if uerr := st.UnstampAppExpiryStop(ctx, e.AppName, stamp); uerr != nil {
				err = errors.Join(err, uerr)
			}
			return false, err
		}
		rt.changes.Publish(changeApps, e.AppName)
		rt.nudgeReconciler()
	}
	rt.recordAppExpiryAudit(ctx, "app.expiry.stop", e.AppName, stoppedBy)
	return true, nil
}

// deleteExpiredApp claims the row and deletes the app under one name lock,
// re-checking identity and every keep rule on the re-read app.
func (rt *Router) deleteExpiredApp(ctx context.Context, st appExpiryStore, e store.AppExpiry) (bool, error) {
	var volumes []string
	claimedAt := ""
	kept := false
	teardownErr, err := rt.deleteAppGuarded(ctx, e.AppName, func(cur store.DesiredService) error {
		if !rt.appExpiryDeleteEnabled(ctx) || cur.Incarnation != e.Incarnation {
			return errAppExpiryNotClaimed
		}
		reason, kerr := rt.appExpiryKeepReason(ctx, cur, e)
		if kerr != nil {
			return kerr
		}
		if reason != "" {
			if _, err := rt.keepAppExpiry(ctx, st, e, reason); err != nil {
				return err
			}
			kept = true
			return errAppExpiryNotClaimed
		}
		stamp := time.Now().UTC().Format(time.RFC3339Nano)
		ok, cerr := st.ClaimAppExpiry(ctx, e, stamp)
		if cerr != nil {
			return cerr
		}
		if !ok {
			return errAppExpiryNotClaimed
		}
		claimedAt = stamp
		if e.DeleteData && rt.isLocalNode(cur.NodeID) {
			for _, v := range cur.Volumes {
				if v.Name != deploy.NextImageCacheVolume(cur.Name) {
					volumes = append(volumes, v.Name)
				}
			}
		}
		rt.recordAppExpiryAudit(ctx, "app.expiry.delete.begin", e.AppName, "")
		return nil
	})
	if errors.Is(err, errAppExpiryNotClaimed) {
		return kept, nil
	}
	if err != nil {
		if claimedAt != "" {
			rt.recordAppExpiryAudit(ctx, "app.expiry.delete.failed", e.AppName, "")
			if rerr := st.ReleaseAppExpiryClaim(ctx, e.AppName, claimedAt); rerr != nil {
				rt.logger.Error("api: app expiry: release claim failed", slog.String("name", e.AppName), slog.String("error", rerr.Error()))
				err = errors.Join(err, rerr)
			}
		}
		return false, err
	}
	if len(volumes) > 0 && teardownErr == nil {
		rt.purgeVolumes(context.WithoutCancel(ctx), "app/"+e.AppName, volumes)
	}
	rt.changes.Publish(changeApps, e.AppName)
	rt.nudgeReconciler()
	rt.recordAppExpiryAudit(ctx, "app.expiry.delete", e.AppName, "")
	return true, nil
}
