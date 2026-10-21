package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/setupguide"
	"github.com/GLINCKER/levelrail/internal/store"
)

// SetupGuideStore persists the Get started hub's dismissal and step overrides.
type SetupGuideStore interface {
	GetSetupGuideState(ctx context.Context) (store.SetupGuideState, error)
	SetSetupGuideHidden(ctx context.Context, hidden bool, by string) error
	SetSetupGuideOverride(ctx context.Context, stepID, state, by string) error
	ResetSetupGuide(ctx context.Context) error
}

type setupGuideStepRequest struct {
	State string `json:"state"`
}

type setupGuideVisibilityRequest struct {
	Hidden bool `json:"hidden"`
}

func (rt *Router) setupGuideOr501(w http.ResponseWriter) (SetupGuideStore, bool) {
	if rt.setupGuide == nil {
		writeError(w, http.StatusNotImplemented, "the setup guide is not available on this control plane")
		return nil, false
	}
	return rt.setupGuide, true
}

func (rt *Router) handleGetSetupGuide(w http.ResponseWriter, r *http.Request) {
	rt.writeSetupGuide(w, r)
}

func (rt *Router) writeSetupGuide(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.setupGuideOr501(w)
	if !ok {
		return
	}
	state, err := st.GetSetupGuideState(r.Context())
	if err != nil {
		rt.internalError(w, "api: get setup guide state failed", err)
		return
	}
	in := rt.collectSetupGuideInput(r)
	in.Hidden = state.Hidden
	in.Overrides = make(map[setupguide.StepID]string, len(state.Overrides))
	for id, v := range state.Overrides {
		in.Overrides[setupguide.StepID(id)] = v
	}
	writeJSON(w, http.StatusOK, setupguide.Derive(in))
}

func (rt *Router) handleSetSetupGuideStep(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.setupGuideOr501(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !setupguide.ValidStep(id) {
		writeError(w, http.StatusNotFound, "unknown setup step")
		return
	}
	var req setupGuideStepRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	override := ""
	switch req.State {
	case "todo":
	case setupguide.OverrideSkipped, setupguide.OverrideDone:
		override = req.State
	default:
		writeError(w, http.StatusBadRequest, `state must be "todo", "skipped" or "done"`)
		return
	}
	if err := st.SetSetupGuideOverride(r.Context(), id, override, rt.setupGuideActor(r)); err != nil {
		rt.internalError(w, "api: set setup guide step failed", err, slog.String("step", id))
		return
	}
	rt.writeSetupGuide(w, r)
}

func (rt *Router) handleSetSetupGuideVisibility(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.setupGuideOr501(w)
	if !ok {
		return
	}
	var req setupGuideVisibilityRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := st.SetSetupGuideHidden(r.Context(), req.Hidden, rt.setupGuideActor(r)); err != nil {
		rt.internalError(w, "api: set setup guide visibility failed", err)
		return
	}
	rt.writeSetupGuide(w, r)
}

func (rt *Router) handleResetSetupGuide(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.setupGuideOr501(w)
	if !ok {
		return
	}
	if err := st.ResetSetupGuide(r.Context()); err != nil {
		rt.internalError(w, "api: reset setup guide failed", err)
		return
	}
	rt.writeSetupGuide(w, r)
}

func (rt *Router) setupGuideActor(r *http.Request) string {
	_, id, _, err := rt.callerPrincipal(r)
	if err != nil {
		return ""
	}
	return id
}

// collectSetupGuideInput reads live state. A source that cannot be read marks
// its steps unavailable instead of failing the whole guide.
func (rt *Router) collectSetupGuideInput(r *http.Request) setupguide.Input {
	ctx := r.Context()
	in := setupguide.Input{Unavailable: map[setupguide.StepID]bool{}}
	fail := func(what string, err error, steps ...setupguide.StepID) {
		rt.logger.Warn("api: setup guide: source unavailable", slog.String("source", what), slog.String("error", err.Error()))
		for _, s := range steps {
			in.Unavailable[s] = true
		}
	}
	unavailable := func(steps ...setupguide.StepID) {
		for _, s := range steps {
			in.Unavailable[s] = true
		}
	}

	host, _ := advertisedHost(r, rt.publicHost)
	in.ServerAddress = host

	ing, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		fail("ingress settings", err, setupguide.StepTLS)
	} else {
		in.ACMEEnabled, in.BehindProxy = ing.ACMEEnabled, ing.TLSTerminatedUpstream
	}
	if u, err := rt.ingressSettings.GetDashboardURL(ctx); err != nil {
		fail("dashboard url", err, setupguide.StepPublicAddress)
	} else {
		in.DashboardURL = u
	}

	rt.collectSetupGuideSecurity(r, &in, unavailable)
	rt.collectSetupGuideSignInRecovery(r.Context(), &in)
	rt.collectSetupGuideSource(ctx, &in)
	rt.collectSetupGuideTraffic(ctx, &in, fail)
	rt.collectSetupGuideOps(ctx, &in, fail, unavailable)
	return in
}

func (rt *Router) collectSetupGuideSecurity(r *http.Request, in *setupguide.Input, unavailable func(...setupguide.StepID)) {
	ctx := r.Context()
	if rt.mfaLib == nil {
		unavailable(setupguide.StepSecureAdmin)
		return
	}
	candidates := []string{}
	if ptype, id, _, err := rt.callerPrincipal(r); err == nil && ptype == store.PrincipalTypeUser {
		candidates = append(candidates, id)
	} else if users, err := rt.auth.ListUsers(ctx); err == nil {
		for _, u := range users {
			candidates = append(candidates, u.ID)
		}
	}
	for _, id := range candidates {
		if rt.userHasSecondFactor(ctx, id) {
			in.SecuredAdmin = true
			return
		}
	}
}

// collectSetupGuideSignInRecovery marks the sign-in recovery step satisfied
// when the server can send email or a live token can approve a new browser.
func (rt *Router) collectSetupGuideSignInRecovery(ctx context.Context, in *setupguide.Input) {
	in.EmailConfigured = rt.emailReady(ctx)
	tokens, err := rt.tokens.ListAPITokens(ctx)
	if err != nil {
		rt.logger.Warn("api: setup guide: list tokens failed", slog.String("error", err.Error()))
		return
	}
	now := time.Now()
	for _, t := range tokens {
		if t.RevokedAt != nil || (t.ExpiresAt != nil && t.ExpiresAt.Before(now)) {
			continue
		}
		if slices.Contains(t.Abilities, AbilitySignInApprove) {
			in.SignInApprover = true
			return
		}
	}
}

func (rt *Router) userHasSecondFactor(ctx context.Context, userID string) bool {
	m := rt.mfaLib.mfa
	if m.TOTPAvailable() {
		st, err := m.TOTPStatus(ctx, userID)
		if err != nil && !errors.Is(err, authengine.ErrNotMapped) {
			rt.logger.Warn("api: setup guide: totp status failed", slog.String("user_id", userID), slog.String("error", err.Error()))
		}
		if err == nil && st.Enabled {
			return true
		}
	}
	keys, err := m.PasskeyList(ctx, userID)
	if err != nil && !errors.Is(err, authengine.ErrNotMapped) {
		rt.logger.Warn("api: setup guide: passkey list failed", slog.String("user_id", userID), slog.String("error", err.Error()))
	}
	return err == nil && len(keys) > 0
}

func (rt *Router) collectSetupGuideSource(ctx context.Context, in *setupguide.Input) {
	statuses := []gitProviderResource{rt.githubProviderStatus(ctx)}
	for _, p := range connectionGitProviders {
		statuses = append(statuses, rt.gitProviderStatusFor(ctx, p))
	}
	for _, p := range statuses {
		if p.Connected {
			in.GitConnected = true
		}
	}
	if apps, err := rt.apps.ListDesiredServices(ctx); err != nil {
		rt.logger.Warn("api: setup guide: list apps failed", slog.String("error", err.Error()))
		in.Unavailable[setupguide.StepFirstApp] = true
	} else {
		in.Apps = len(apps)
	}
}

func (rt *Router) collectSetupGuideTraffic(ctx context.Context, in *setupguide.Input, fail func(string, error, ...setupguide.StepID)) {
	rows, err := rt.domains.ListServiceDomains(ctx)
	if err != nil {
		fail("domains", err, setupguide.StepAppDomain, setupguide.StepTLS)
	} else {
		window := rt.certExpiryWarningWindow
		if window <= 0 {
			window = alerting.DefaultCertExpiryWarningWindow
		}
		certs, cerr := alerting.ListCertificates(ctx, rt.certs, window, time.Now(), rt.logger)
		if cerr != nil {
			fail("certificates", cerr, setupguide.StepTLS)
		}
		for _, d := range rows {
			in.Domains = append(in.Domains, setupguide.DomainTLS{Name: d.Domain, CertHealthy: certCovers(certs, d.Domain)})
		}
	}

	cf, err := rt.cloudflareDNS.GetCloudflareDNSSettings(ctx)
	if err != nil {
		fail("cloudflare dns", err, setupguide.StepDNS)
	} else {
		in.DNSProviderConnected = cf.Enabled
	}
	if r53, err := rt.route53DNS.GetRoute53DNSSettings(ctx); err == nil && r53.Enabled {
		in.DNSProviderConnected = true
	}
}

func (rt *Router) collectSetupGuideOps(ctx context.Context, in *setupguide.Input, fail func(string, error, ...setupguide.StepID), unavailable func(...setupguide.StepID)) {
	if targets, err := rt.backupTargets.ListBackupTargets(ctx); err != nil {
		fail("backup targets", err, setupguide.StepBackupTarget)
	} else {
		in.BackupTargets = len(targets)
	}
	if rt.cpBackups == nil {
		unavailable(setupguide.StepFirstBackup, setupguide.StepVerifiedBackup)
	} else if list, err := rt.cpBackups.List(); err != nil {
		fail("control plane backups", err, setupguide.StepFirstBackup, setupguide.StepVerifiedBackup)
	} else {
		in.ControlPlaneBackups = len(list)
		for _, b := range list {
			if b.VerifiedOK != nil && *b.VerifiedOK {
				in.VerifiedBackups++
			}
		}
	}

	if rt.notificationChannels == nil || rt.alertRules == nil {
		unavailable(setupguide.StepNotifications)
	} else if chans, err := rt.notificationChannels.ListNotificationChannels(ctx); err != nil {
		fail("notification channels", err, setupguide.StepNotifications)
	} else {
		in.Channels = len(chans)
		in.AlertRules = rt.countAlertRules(ctx)
	}

	if users, err := rt.auth.ListUsers(ctx); err != nil {
		fail("users", err, setupguide.StepTeammate)
	} else {
		in.Users = len(users)
		if inv, err := rt.invites.ListPendingInvites(ctx); err == nil {
			in.PendingInvites = len(inv)
		}
	}

	rt.collectSetupGuideUpgrades(ctx, in, unavailable)
}

func (rt *Router) collectSetupGuideUpgrades(ctx context.Context, in *setupguide.Input, unavailable func(...setupguide.StepID)) {
	known := false
	if rt.upgradeSchedule != nil {
		if p, err := rt.upgradeSchedule.store.GetUpgradePolicy(ctx); err == nil {
			known = true
			in.AutomaticUpgrades = p.Mode != "" && p.Mode != "off"
		}
	}
	if rt.updateSettings != nil {
		if s, err := rt.updateSettings.GetUpdateSettings(ctx); err == nil {
			known = true
			in.AutomaticUpgrades = in.AutomaticUpgrades || s.AutoUpdateEnabled
		}
	}
	if !known {
		unavailable(setupguide.StepUpgrades)
	}
}

// countAlertRules counts rules across every app: platform-wide rules hang off
// an app resource, so a rule on any app proves alerting is wired.
func (rt *Router) countAlertRules(ctx context.Context) int {
	apps, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return 0
	}
	total := 0
	for _, a := range apps {
		rules, err := rt.alertRules.ListRulesForResource(ctx, resourceIDForApp(a.Name))
		if err != nil {
			continue
		}
		total += len(rules)
		if total > 0 {
			break
		}
	}
	return total
}

func certCovers(certs []alerting.CertInfo, domain string) bool {
	for _, c := range certs {
		if c.Status != "healthy" {
			continue
		}
		names := append([]string{c.Domain}, c.SANs...)
		for _, n := range names {
			if n == domain {
				return true
			}
			if strings.HasPrefix(n, "*.") && strings.HasSuffix(domain, n[1:]) && !strings.Contains(strings.TrimSuffix(domain, n[1:]), ".") {
				return true
			}
		}
	}
	return false
}
