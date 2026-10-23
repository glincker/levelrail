package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/fixit"
)

// SubjectSettings groups the fixes the Settings health card lists.
const SubjectSettings = "settings"

const dashboardURLProbeTimeout = 6 * time.Second

func (rt *Router) settingsFixes() []fixit.Fix {
	return []fixit.Fix{
		rt.dashboardHTTPSFix(),
		rt.webhookPublicURLFix(),
		rt.passkeyRPFix(),
		rt.trustedProxiesFix(),
		rt.guidedSettingsFix("settings.email.sender", "email_sender", "Set up an email sender",
			"Invites, password resets and alerts cannot be delivered until a sender is configured.",
			"Open Settings, Email, choose a backend and send a test message, then re-check.", rt.checkEmailSender),
		rt.guidedSettingsFix("settings.backups.enabled", "backups_configured", "Turn on control plane backups",
			"Without scheduled snapshots a lost disk loses every app definition, secret and user.",
			"Open Settings, Control plane backup, and keep the schedule on, then re-check.", rt.checkBackupsConfigured),
		rt.guidedSettingsFix("settings.admin.second", "second_admin", "Add a second administrator",
			"With one administrator, a lost password or passkey locks everyone out of the dashboard.",
			"Open Settings, Users, invite a second person with the root ability, then re-check.", rt.checkSecondAdmin),
		rt.guidedSettingsFix("settings.master_key.age", "master_key_age", "Rotate the master key",
			"An old master key widens the window in which a leaked copy still decrypts every secret.",
			"Open Settings, General, Master key, and rotate it, then re-check.", rt.checkMasterKeyAge),
	}
}

func (rt *Router) guidedSettingsFix(id, check, title, why, reason string, c func(context.Context) (bool, string)) fixit.Fix {
	return fixit.GuidedCheckFix(fixit.GuidedDeps{
		ID: id, CheckID: check, Subject: SubjectSettings, Title: title, Why: why, Risk: fixit.RiskSafe,
		Check: func(ctx context.Context) (fixit.Finding, error) {
			failing, detail := c(ctx)
			return fixit.Finding{Failing: failing, Detail: detail, Reason: reason}, nil
		},
	})
}

func (rt *Router) checkEmailSender(ctx context.Context) (bool, string) {
	if rt.emailSettings == nil {
		return false, "email settings are not available"
	}
	s, err := rt.emailSettings.GetEmailSettings(ctx)
	if err != nil {
		return false, "could not read email settings"
	}
	if s.Backend == "" {
		return true, "no email backend is configured"
	}
	return false, "email backend: " + s.Backend
}

func (rt *Router) checkBackupsConfigured(context.Context) (bool, string) {
	switch {
	case rt.cpBackups == nil:
		return true, "control plane backups are not configured"
	case rt.cpBackupScheduleOff:
		return true, "scheduled backups are off"
	}
	return false, "scheduled backups are on"
}

func (rt *Router) checkSecondAdmin(ctx context.Context) (bool, string) {
	if rt.auth == nil {
		return false, "user list is not available"
	}
	users, err := rt.auth.ListUsers(ctx)
	if err != nil {
		return false, "could not list users"
	}
	n := 0
	for _, u := range users {
		if slices.Contains(u.Abilities, AbilityRoot) {
			n++
		}
	}
	if n < 2 {
		return true, fmt.Sprintf("%d administrator", n)
	}
	return false, fmt.Sprintf("%d administrators", n)
}

func (rt *Router) checkMasterKeyAge(ctx context.Context) (bool, string) {
	c := rt.doctorCheckMasterKeyRotation(ctx)
	return c.Status == doctorStatusWarn, c.Message
}

// effectiveDashboardURL is the saved dashboard URL, else the APP_DASHBOARD_URL pin the sign-in paths already honour.
func (rt *Router) effectiveDashboardURL(ctx context.Context) (string, error) {
	cur, err := rt.ingressSettings.GetDashboardURL(ctx)
	if err != nil || cur != "" {
		return cur, err
	}
	return strings.TrimSpace(os.Getenv(envDashboardURL)), nil
}

// dashboardHTTPSTarget is the https URL the dashboard should use, empty when
// nothing needs fixing (no primary domain, ACME off, or already on it).
func (rt *Router) dashboardHTTPSTarget(ctx context.Context) (target, current string, err error) {
	ing, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return "", "", err
	}
	current, err = rt.effectiveDashboardURL(ctx)
	if err != nil {
		return "", "", err
	}
	if ing.PrimaryDomain == "" || !ing.ACMEEnabled {
		return "", current, nil
	}
	want := "https://" + ing.PrimaryDomain
	if current == want {
		return "", current, nil
	}
	return want, current, nil
}

func (rt *Router) dashboardHTTPSFix() fixit.Fix {
	return fixit.Fix{
		ID: "settings.dashboard_url.https", CheckID: "dashboard_url_https", Subject: SubjectSettings, Kind: fixit.KindAuto,
		Title: "Switch the dashboard URL to https",
		Why:   "Sign-in cookies, passkeys and OAuth callbacks are bound to the dashboard URL. Plain http exposes them on the network.",
		Risk:  fixit.RiskExposes, Reversible: true, Needs: "root",
		Detect: func(ctx context.Context) (fixit.Applicability, error) {
			want, cur, err := rt.dashboardHTTPSTarget(ctx)
			if err != nil {
				return fixit.Applicability{}, err
			}
			if want == "" {
				return fixit.Applicability{Detail: "dashboard URL: " + orNotSet(cur), Changes: []fixit.Change{}}, nil
			}
			return fixit.Applicability{
				Failing: true,
				Detail:  fmt.Sprintf("dashboard URL is %s but the primary domain has certificates on", orNotSet(cur)),
				Changes: []fixit.Change{
					{Label: "Check that " + want + " answers with a valid certificate"},
					{Label: "Save " + want + " as the dashboard URL"},
					{Label: "Plain http sign-in is refused afterwards", Detail: "set APP_ALLOW_INSECURE_LOGIN=true on the server to recover"},
					{Label: "Passkeys keep working on " + want, Detail: "ones registered on another host must be added again"},
					{Label: "OAuth and SSO callbacks move to " + want, Detail: "update the redirect URL at each provider"},
				},
			}, nil
		},
		Run: func(ctx context.Context, _ fixit.Input) error {
			want, _, err := rt.dashboardHTTPSTarget(ctx)
			if err != nil || want == "" {
				return err
			}
			if err := probeDashboardHTTPS(ctx, want); err != nil {
				return err
			}
			return rt.ingressSettings.SetDashboardURL(ctx, want)
		},
	}
}

func orNotSet(u string) string {
	if u == "" {
		return "not set"
	}
	return u
}

// probeDashboardHTTPS proves the https URL reaches this control plane with a
// trusted certificate before the setting can lock plain-http sign-in out.
func probeDashboardHTTPS(ctx context.Context, base string) error {
	ctx, cancel := context.WithTimeout(ctx, dashboardURLProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/brand", nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: dashboardURLProbeTimeout}).Do(req) //nolint:gosec // base is the operator's own primary domain
	if err != nil {
		return fmt.Errorf("%s is not answering over https yet (certificate issued and DNS pointing here?): %w", base, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %d, not this dashboard", base, resp.StatusCode)
	}
	return nil
}

func (rt *Router) passkeyRPFix() fixit.Fix {
	return fixit.GuidedCheckFix(fixit.GuidedDeps{
		ID: "settings.passkeys.rp_id", CheckID: "passkey_rp", Subject: SubjectSettings,
		Title: "Make passkeys match the dashboard host",
		Why:   "A passkey only works on the host it was registered for. This page is served from a different host than the dashboard URL.",
		Risk:  fixit.RiskSafe,
		Check: func(ctx context.Context) (fixit.Finding, error) {
			cur, err := rt.effectiveDashboardURL(ctx)
			if err != nil || cur == "" {
				return fixit.Finding{Detail: "no dashboard URL set"}, nil
			}
			u, perr := url.Parse(cur)
			host, _ := ctx.Value(fixitCtxKey{}).(string)
			if perr != nil || host == "" {
				return fixit.Finding{Detail: "passkey host: " + cur}, nil
			}
			page := strings.ToLower(strings.SplitN(host, ":", 2)[0])
			rp := strings.ToLower(u.Hostname())
			if v := strings.TrimSpace(os.Getenv(authengine.EnvWebAuthnRPID)); v != "" {
				rp = strings.ToLower(v)
			}
			if page == rp || strings.HasSuffix(page, "."+rp) {
				return fixit.Finding{Detail: "passkeys are bound to " + rp}, nil
			}
			return fixit.Finding{
				Failing: true,
				Detail:  fmt.Sprintf("passkeys are bound to %s but you opened %s", rp, page),
				Reason:  "Open the dashboard at " + cur + ", or fix the dashboard URL above, then re-check.",
			}, nil
		},
	})
}
