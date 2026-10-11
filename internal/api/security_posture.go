package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/exposure"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/semver"
)

const (
	envPostureCacheTTL     = "APP_SECURITY_POSTURE_CACHE_TTL"
	envPostureExposureTTL  = "APP_SECURITY_POSTURE_EXPOSURE_TTL"
	envSessionMaxAgeDays   = "APP_SECURITY_SESSION_MAX_AGE_DAYS"
	defaultPostureCacheTTL = 30 * time.Second
	defaultPostureExpTTL   = 5 * time.Minute
	defaultSessionMaxAge   = 7
	postureExposureTimeout = 4 * time.Second
)

type postureCounts struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Unknown  int `json:"unknown"`
	Passing  int `json:"passing"`
}

type postureResponse struct {
	Score       int           `json:"score"`
	Grade       string        `json:"grade"`
	GeneratedAt time.Time     `json:"generated_at"`
	Counts      postureCounts `json:"counts"`
	Full        bool          `json:"full"`
	Items       []postureItem `json:"items"`
	Account     []postureItem `json:"account"`
}

// postureCache holds the last computed platform posture, and separately the
// exposure audit, which may reach remote agents and so refreshes less often.
type postureCache struct {
	mu         sync.Mutex
	at         time.Time
	items      []postureItem
	exposureAt time.Time
	exposure   *postureExposure
}

func (c *postureCache) invalidate() {
	c.mu.Lock()
	c.at = time.Time{}
	c.mu.Unlock()
}

// posture returns the cached item list, recomputing it when stale. The lock
// is held while computing so a burst of requests computes once.
func (rt *Router) posture(ctx context.Context, now time.Time) ([]postureItem, time.Time) {
	c := &rt.sec.posture
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.at.IsZero() && now.Sub(c.at) < envDuration(envPostureCacheTTL, defaultPostureCacheTTL) {
		return c.items, c.at
	}
	if c.exposureAt.IsZero() || now.Sub(c.exposureAt) >= envDuration(envPostureExposureTTL, defaultPostureExpTTL) {
		c.exposure, c.exposureAt = rt.postureExposure(ctx), now
	}
	in := rt.collectPosture(ctx, now)
	in.Exposure = c.exposure
	c.items, c.at = evaluatePosture(in), now
	return c.items, c.at
}

func (rt *Router) postureExposure(ctx context.Context) *postureExposure {
	if rt.exposure == nil || rt.apps == nil || rt.databases == nil {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, postureExposureTimeout)
	defer cancel()
	rep, err := rt.exposureReport(cctx, "", false)
	if err != nil {
		return nil
	}
	out := &postureExposure{}
	for _, n := range rep.Nodes {
		for _, f := range n.Findings {
			if !f.NeedsAttention() || f.Severity == exposure.SeverityInfo {
				continue
			}
			if f.ImageKind == exposure.KindDockerAPI {
				out.DockerAPI++
				continue
			}
			out.Attention++
		}
	}
	return out
}

func (rt *Router) collectPosture(ctx context.Context, now time.Time) postureInput {
	in := postureInput{Now: now, NewDeviceApproval: rt.newDeviceApproval, Anomalies: rt.sec.failures.recent(now)}
	in.Policy, _, _ = rt.effectiveSecurityPolicy(ctx)
	maxAge, _ := envDays(envSessionMaxAgeDays, defaultSessionMaxAge)
	in.SessionMaxAge = time.Duration(maxAge) * hygieneDay
	rt.collectPostureUsers(ctx, &in)
	if toks, err := rt.libraryListTokens(ctx, "", true); err == nil {
		in.TokensKnown = true
		for _, t := range toks {
			in.Tokens = append(in.Tokens, store.APIToken{ID: t.ID, Name: t.Name, Abilities: t.Abilities, CreatedAt: t.CreatedAt,
				LastUsedAt: t.LastUsedAt, ExpiresAt: t.ExpiresAt, RevokedAt: t.RevokedAt})
		}
	}
	if s, err := rt.codeLoginSettings(ctx); err == nil {
		in.CodeLoginAdmins = &s.Admins
	}
	if rt.cpDR != nil {
		if st, err := rt.cpDR.Status(ctx); err == nil {
			on := st.Enabled
			in.OffBoxBackups = &on
		}
	} else {
		off := false
		in.OffBoxBackups = &off
	}
	rt.collectPostureSecrets(ctx, &in, now)
	rt.collectPostureTLS(ctx, &in, now)
	if nodes, err := rt.nodes.ListNodes(ctx); err == nil {
		in.AgentsKnown = true
		for _, n := range nodes {
			if rt.nodeCerts.minAgentVersion == "" {
				break
			}
			if cmp, ok := semver.Compare(n.AgentVersion, rt.nodeCerts.minAgentVersion); n.AgentVersion == "" || (ok && cmp < 0) {
				in.OutdatedAgents = append(in.OutdatedAgents, n.Name)
			}
		}
	}
	return in
}

func (rt *Router) collectPostureUsers(ctx context.Context, in *postureInput) {
	users, err := rt.auth.ListUsers(ctx)
	if err != nil {
		return
	}
	flagged := map[string]bool{}
	if rows, ferr := rt.security.ListUserSecuritySettings(ctx); ferr == nil {
		for _, s := range rows {
			flagged[s.UserID] = !s.ResetFlaggedAt.IsZero()
		}
	}
	var mfa *authengine.MFA
	if rt.mfaLib != nil {
		mfa = rt.mfaLib.mfa
	}
	in.MFAAvailable = mfa != nil && (mfa.TOTPAvailable() || mfa.PasskeysAvailable())
	in.UsersKnown, in.SessionsKnown = true, rt.libSessions != nil
	for _, u := range users {
		pu := postureUser{ID: u.ID, Name: u.Email, Admin: hasAbility(u.Abilities, AbilityRoot), Flagged: flagged[u.ID]}
		if mfa != nil && mfa.TOTPAvailable() {
			st, serr := mfa.TOTPStatus(ctx, u.ID)
			switch {
			case serr == nil:
				pu.TOTP, pu.RecoveryCodes = st.Enabled, st.RecoveryCodesRemaining
			case !errors.Is(serr, authengine.ErrNotMapped):
				in.UsersKnown = false
			}
		}
		if mfa != nil && mfa.PasskeysAvailable() {
			if pk, perr := mfa.PasskeyList(ctx, u.ID); perr == nil {
				pu.Passkeys = len(pk)
			}
		}
		in.Users = append(in.Users, pu)
		if rt.libSessions == nil {
			continue
		}
		views, serr := rt.libSessions.ListUserSessions(ctx, u.ID, "")
		if serr != nil {
			in.SessionsKnown = false
			continue
		}
		for _, v := range views {
			in.Sessions = append(in.Sessions, postureSession{UserID: u.ID, CreatedAt: v.CreatedAt})
		}
	}
}

func (rt *Router) collectPostureSecrets(ctx context.Context, in *postureInput, now time.Time) {
	in.MasterKey = rt.masterKeyRotator != nil
	in.MasterKeyMaxAge = rt.doctorMasterKeyRotationWarnAge
	if in.MasterKeyMaxAge <= 0 {
		in.MasterKeyMaxAge = defaultDoctorMasterKeyRotationWarnAge
	}
	if rt.masterKeyRotator != nil {
		if at, ok, err := rt.masterKeyRotator.GetMasterKeyRotatedAt(ctx); err == nil {
			in.MasterKeyKnown = true
			if ok {
				in.MasterKeyRotated = &at
			}
		}
	}
	age := rt.effectiveSecretRotationWarnAge()
	in.SecretMaxAgeDays = int(age / hygieneDay)
	if rt.staleSecretCounter != nil {
		if n, err := rt.staleSecretCounter.CountStaleSecrets(ctx, now.UTC().Add(-age)); err == nil {
			in.StaleSecrets = &n
		}
	}
}

// collectPostureTLS finds routed domains served without a publicly trusted
// certificate: everything while ACME is off, otherwise domains whose only
// certificate is missing or from the internal issuer.
func (rt *Router) collectPostureTLS(ctx context.Context, in *postureInput, now time.Time) {
	s, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return
	}
	domains, err := rt.domains.ListServiceDomains(ctx)
	if err != nil {
		return
	}
	certs, err := alerting.ListCertificates(ctx, rt.certs, alerting.DefaultCertExpiryWarningWindow, now, rt.logger)
	if err != nil {
		return
	}
	in.TLSKnown, in.TLSUpstream, in.ACMEEnabled = true, s.TLSTerminatedUpstream, s.EffectiveACMEEnabled()
	in.HSTS = rt.hstsEnabled || rt.hstsEnabledFromDB(ctx)
	public := map[string]bool{}
	for _, c := range certs {
		if isInternalIssuer(c.Issuer) || !c.NotAfter.After(now) {
			continue
		}
		in.RealCert = true
		public[strings.ToLower(c.Domain)] = true
		for _, san := range c.SANs {
			public[strings.ToLower(san)] = true
		}
	}
	names := []string{}
	if s.PrimaryDomain != "" {
		names = append(names, s.PrimaryDomain)
	}
	for _, d := range domains {
		names = append(names, d.Domain)
	}
	seen := map[string]bool{}
	for _, d := range names {
		key := strings.ToLower(d)
		if seen[key] || public[key] {
			continue
		}
		seen[key] = true
		in.InsecureDomains = append(in.InsecureDomains, d)
	}
}

// accountPosture is the caller's own checklist, computed per request from a
// handful of reads about one account.
func (rt *Router) accountPosture(ctx context.Context, userID string) []postureItem {
	in := postureInput{Now: time.Now(), UsersKnown: true}
	u, err := rt.auth.GetUserByID(ctx, userID)
	if err != nil {
		return []postureItem{}
	}
	pu := postureUser{ID: u.ID, Name: u.Email, Admin: true}
	if rt.mfaLib != nil && rt.mfaLib.mfa != nil {
		if on, terr := rt.totpEnabled(ctx, u.ID); terr == nil && on {
			pu.TOTP = true
			if st, serr := rt.mfaLib.mfa.TOTPStatus(ctx, u.ID); serr == nil {
				pu.RecoveryCodes = st.RecoveryCodesRemaining
			}
		}
		if rt.mfaLib.mfa.PasskeysAvailable() {
			if pk, perr := rt.mfaLib.mfa.PasskeyList(ctx, u.ID); perr == nil {
				pu.Passkeys = len(pk)
			}
		}
	}
	if s, serr := rt.security.GetUserSecuritySettings(ctx, u.ID); serr == nil {
		pu.Flagged = !s.ResetFlaggedAt.IsZero()
	}
	in.Users = []postureUser{pu}
	link := linkFix("/settings/security")
	items := []postureItem{
		userRule(in, "own_mfa", postureHigh, link, func(u postureUser) bool { return !u.TOTP && u.Passkeys == 0 }),
		userRule(in, "own_recovery_codes", postureMedium, link, func(u postureUser) bool { return u.TOTP && u.RecoveryCodes == 0 }),
		userRule(in, "own_account_flagged", postureCritical, link, func(u postureUser) bool { return u.Flagged }),
	}
	for i := range items {
		items[i].Subjects = nil
	}
	return items
}

func countPosture(items []postureItem) postureCounts {
	var c postureCounts
	for _, it := range items {
		switch {
		case it.Status == postureStatusPass:
			c.Passing++
		case it.Status == postureStatusUnknown:
			c.Unknown++
		case it.Severity == postureCritical:
			c.Critical++
		case it.Severity == postureHigh:
			c.High++
		case it.Severity == postureMedium:
			c.Medium++
		default:
			c.Low++
		}
	}
	return c
}

// handleSecurityPosture handles GET /api/v1/security/posture. Any reader
// gets the score and counts; only an admin gets the item list with the
// names of the accounts, tokens and domains behind it.
func (rt *Router) handleSecurityPosture(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, at := rt.posture(ctx, time.Now())
	score := postureScore(items)
	out := postureResponse{Score: score, Grade: postureGrade(score), GeneratedAt: at, Counts: countPosture(items), Items: []postureItem{}, Account: []postureItem{}}
	if rt.callerIsAdmin(r) {
		out.Full, out.Items = true, items
	}
	if userID, ok := rt.currentSessionUserID(r); ok {
		out.Account = rt.accountPosture(ctx, userID)
	}
	writeJSON(w, http.StatusOK, out)
}

// flaggedAccountItems raises an attention item for each account flagged by
// "this wasn't me": the caller's own, or every one for an admin.
func (rt *Router) flaggedAccountItems(r *http.Request, abilities []string) []attention.Item {
	if rt.security == nil {
		return nil
	}
	admin := hasAbility(abilities, AbilityRoot)
	callerID, _ := rt.currentSessionUserID(r)
	rows, err := rt.security.ListUserSecuritySettings(r.Context())
	if err != nil {
		return nil
	}
	var items []attention.Item
	for _, s := range rows {
		if s.ResetFlaggedAt.IsZero() || (!admin && s.UserID != callerID) {
			continue
		}
		name := s.UserID
		if u, uerr := rt.auth.GetUserByID(r.Context(), s.UserID); uerr == nil {
			name = u.Email
		}
		it := feedItem(attention.Critical, attention.KindAccountFlagged, name, "a sign-in was reported as not the owner's",
			map[string]string{"name": name, "at": s.ResetFlaggedAt.UTC().Format(time.RFC3339)})
		items = append(items, it)
	}
	return items
}
