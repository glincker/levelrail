package api

import (
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Posture severities, highest first, and what a failing item costs the score.
const (
	postureCritical = "critical"
	postureHigh     = "high"
	postureMedium   = "medium"
	postureLow      = "low"

	postureStatusPass    = "pass"
	postureStatusFail    = "fail"
	postureStatusUnknown = "unknown"

	postureFixLink   = "link"
	postureFixAction = "action"
)

var postureWeights = map[string]int{postureCritical: 25, postureHigh: 15, postureMedium: 8, postureLow: 3}

var postureRank = map[string]int{postureCritical: 0, postureHigh: 1, postureMedium: 2, postureLow: 3}

type postureFix struct {
	Kind   string            `json:"kind"`
	Link   string            `json:"link,omitempty"`
	Action string            `json:"action,omitempty"`
	Params map[string]string `json:"params,omitempty"`
	CLI    string            `json:"cli,omitempty"`
}

type postureItem struct {
	ID       string      `json:"id"`
	Severity string      `json:"severity"`
	Status   string      `json:"status"`
	Count    int         `json:"count"`
	Subjects []string    `json:"subjects,omitempty"`
	Detail   string      `json:"detail,omitempty"`
	Fix      *postureFix `json:"fix,omitempty"`
}

type postureUser struct {
	ID, Name      string
	Admin         bool
	TOTP          bool
	RecoveryCodes int
	Passkeys      int
	Flagged       bool
}

type postureSession struct {
	UserID    string
	CreatedAt time.Time
}

type postureExposure struct {
	Attention int
	DockerAPI int
}

// postureInput is every fact the rules read, gathered once per cache period.
// A nil pointer or a false *Known means the source could not be read.
type postureInput struct {
	Now               time.Time
	Policy            securityPolicy
	MFAAvailable      bool
	Users             []postureUser
	UsersKnown        bool
	Tokens            []store.APIToken
	TokensKnown       bool
	Sessions          []postureSession
	SessionsKnown     bool
	SessionMaxAge     time.Duration
	NewDeviceApproval bool
	CodeLoginAdmins   *bool
	Exposure          *postureExposure
	OffBoxBackups     *bool
	MasterKey         bool
	MasterKeyRotated  *time.Time
	MasterKeyKnown    bool
	MasterKeyMaxAge   time.Duration
	StaleSecrets      *int
	SecretMaxAgeDays  int
	TLSKnown          bool
	TLSUpstream       bool
	ACMEEnabled       bool
	InsecureDomains   []string
	RealCert          bool
	HSTS              bool
	OutdatedAgents    []string
	AgentsKnown       bool
	Anomalies         []loginAnomaly
}

func pass(id, sev string) postureItem {
	return postureItem{ID: id, Severity: sev, Status: postureStatusPass}
}

func unknown(id, sev, detail string) postureItem {
	return postureItem{ID: id, Severity: sev, Status: postureStatusUnknown, Detail: detail}
}

func failing(id, sev string, subjects []string, fix *postureFix) postureItem {
	sort.Strings(subjects)
	return postureItem{ID: id, Severity: sev, Status: postureStatusFail, Count: len(subjects), Subjects: subjects, Fix: fix}
}

func linkFix(path string) *postureFix { return &postureFix{Kind: postureFixLink, Link: path} }

func actionFix(action string, params map[string]string, link string) *postureFix {
	return &postureFix{Kind: postureFixAction, Action: action, Params: params, Link: link}
}

func liveTokens(in postureInput) []store.APIToken {
	var out []store.APIToken
	for _, t := range in.Tokens {
		if t.RevokedAt == nil && (t.ExpiresAt == nil || t.ExpiresAt.After(in.Now)) {
			out = append(out, t)
		}
	}
	return out
}

// tokenRule fails for every live token match reports.
func tokenRule(in postureInput, id, sev string, fix *postureFix, match func(store.APIToken) bool) postureItem {
	if !in.TokensKnown {
		return unknown(id, sev, "tokens could not be read")
	}
	var names []string
	for _, t := range liveTokens(in) {
		if match(t) {
			names = append(names, t.Name)
		}
	}
	if len(names) == 0 {
		return pass(id, sev)
	}
	return failing(id, sev, names, fix)
}

// userRule fails for every user match reports.
func userRule(in postureInput, id, sev string, fix *postureFix, match func(postureUser) bool) postureItem {
	if !in.UsersKnown {
		return unknown(id, sev, "accounts could not be read")
	}
	var names []string
	for _, u := range in.Users {
		if match(u) {
			names = append(names, u.Name)
		}
	}
	if len(names) == 0 {
		return pass(id, sev)
	}
	return failing(id, sev, names, fix)
}

func boolRule(id, sev string, known *bool, bad bool, fix *postureFix) postureItem {
	switch {
	case known == nil:
		return unknown(id, sev, "could not be read")
	case *known == bad:
		return failing(id, sev, nil, fix)
	}
	return pass(id, sev)
}

// evaluatePosture runs every rule. Pure, so each rule is table tested.
func evaluatePosture(in postureInput) []postureItem {
	securityPage := "/settings/security"
	tokensPage := "/settings/tokens"
	unusedFor := time.Duration(in.Policy.WarnUnusedDays) * hygieneDay
	items := []postureItem{
		userRule(in, "admin_without_mfa", postureCritical, linkFix(securityPage), func(u postureUser) bool {
			return u.Admin && !u.TOTP && u.Passkeys == 0
		}),
		userRule(in, "account_flagged", postureCritical, linkFix(securityPage), func(u postureUser) bool { return u.Flagged }),
		userRule(in, "recovery_codes_missing", postureMedium, linkFix(securityPage), func(u postureUser) bool {
			return u.TOTP && u.RecoveryCodes == 0
		}),
		passkeysRule(in),
		tokenRule(in, "token_root", postureHigh, linkFix(tokensPage), func(t store.APIToken) bool { return hasAbility(t.Abilities, AbilityRoot) }),
		tokenRule(in, "token_no_expiry", postureMedium, actionFix("set_policy", map[string]string{"max_token_lifetime_days": "90"}, tokensPage),
			func(t store.APIToken) bool { return t.ExpiresAt == nil }),
		tokenRule(in, "token_signin_approve", postureMedium, linkFix(tokensPage), func(t store.APIToken) bool {
			return slices.Contains(t.Abilities, AbilitySignInApprove)
		}),
		tokenRule(in, "token_unused", postureLow, linkFix(tokensPage), func(t store.APIToken) bool {
			return unusedFor > 0 && in.Now.Sub(tokenLastActive(t)) >= unusedFor
		}),
		sessionsRule(in),
		boolRule("new_device_approval_off", postureHigh, &in.NewDeviceApproval, false, &postureFix{Kind: postureFixLink, Link: securityPage, CLI: envNewDeviceApproval + "=true"}),
		approvalScopeRule(in),
		boolRule("code_login_admins", postureMedium, in.CodeLoginAdmins, true, actionFix("disable_code_login_admins", nil, securityPage)),
		exposureRule(in, false),
		exposureRule(in, true),
		boolRule("offbox_backup_off", postureHigh, in.OffBoxBackups, false, &postureFix{Kind: postureFixLink, Link: "/settings/control-plane-backup", CLI: "control-plane-backups schedule show"}),
		masterKeyRule(in),
		staleSecretsRule(in),
		tlsRule(in),
		hstsRule(in),
		agentsRule(in),
		anomalyRule(in),
	}
	sort.SliceStable(items, func(i, j int) bool {
		if (items[i].Status == postureStatusFail) != (items[j].Status == postureStatusFail) {
			return items[i].Status == postureStatusFail
		}
		return postureRank[items[i].Severity] < postureRank[items[j].Severity]
	})
	return items
}

func passkeysRule(in postureInput) postureItem {
	const id = "no_admin_passkey"
	if !in.UsersKnown {
		return unknown(id, postureLow, "accounts could not be read")
	}
	total := 0
	for _, u := range in.Users {
		if u.Admin {
			total += u.Passkeys
		}
	}
	if total > 0 {
		it := pass(id, postureLow)
		it.Count = total
		return it
	}
	return failing(id, postureLow, nil, linkFix("/settings/security"))
}

func sessionsRule(in postureInput) postureItem {
	const id = "sessions_old"
	if !in.SessionsKnown {
		return unknown(id, postureLow, "sessions could not be read")
	}
	var old []string
	for _, s := range in.Sessions {
		if in.SessionMaxAge > 0 && in.Now.Sub(s.CreatedAt) >= in.SessionMaxAge {
			old = append(old, s.UserID)
		}
	}
	if len(old) == 0 {
		return pass(id, postureLow)
	}
	it := failing(id, postureLow, nil, actionFix("review_sessions", nil, "/security?tab=sessions"))
	it.Count = len(old)
	return it
}

func approvalScopeRule(in postureInput) postureItem {
	const id = "approval_password_only"
	if !in.NewDeviceApproval || in.Policy.ApprovalScope == approvalScopeAllMethods {
		return pass(id, postureLow)
	}
	return failing(id, postureLow, nil, actionFix("set_policy", map[string]string{"approval_scope": approvalScopeAllMethods}, ""))
}

func exposureRule(in postureInput, dockerAPI bool) postureItem {
	id, sev := "public_exposure", postureHigh
	if dockerAPI {
		id, sev = "docker_api_exposed", postureCritical
	}
	if in.Exposure == nil {
		return unknown(id, sev, "the exposure audit could not run")
	}
	n := in.Exposure.Attention
	if dockerAPI {
		n = in.Exposure.DockerAPI
	}
	if n == 0 {
		return pass(id, sev)
	}
	it := failing(id, sev, nil, &postureFix{Kind: postureFixLink, Link: "/settings/firewall", CLI: "firewall exposure"})
	it.Count = n
	return it
}

func masterKeyRule(in postureInput) postureItem {
	const id = "master_key_rotation"
	switch {
	case !in.MasterKey:
		return failing(id, postureLow, nil, linkFix("/settings/general"))
	case !in.MasterKeyKnown:
		return unknown(id, postureLow, "rotation history could not be read")
	case in.MasterKeyRotated == nil || in.Now.Sub(*in.MasterKeyRotated) > in.MasterKeyMaxAge:
		return failing(id, postureLow, nil, &postureFix{Kind: postureFixLink, Link: "/settings/general", CLI: "secrets rotate-master-key --new-key-file PATH"})
	}
	return pass(id, postureLow)
}

func staleSecretsRule(in postureInput) postureItem {
	const id = "secrets_old"
	if in.StaleSecrets == nil {
		return unknown(id, postureLow, "secrets could not be counted")
	}
	if *in.StaleSecrets == 0 {
		return pass(id, postureLow)
	}
	it := failing(id, postureLow, nil, linkFix("/apps"))
	it.Count, it.Detail = *in.StaleSecrets, fmt.Sprintf("%d days", in.SecretMaxAgeDays)
	return it
}

func tlsRule(in postureInput) postureItem {
	const id = "tls_not_public"
	switch {
	case !in.TLSKnown:
		return unknown(id, postureHigh, "certificates could not be read")
	case in.TLSUpstream || len(in.InsecureDomains) == 0:
		return pass(id, postureHigh)
	}
	return failing(id, postureHigh, slices.Clone(in.InsecureDomains), linkFix("/domains"))
}

func hstsRule(in postureInput) postureItem {
	const id = "hsts_off"
	if !in.TLSKnown {
		return unknown(id, postureMedium, "certificates could not be read")
	}
	if !in.RealCert || in.HSTS {
		return pass(id, postureMedium)
	}
	return failing(id, postureMedium, nil, linkFix("/domains"))
}

func agentsRule(in postureInput) postureItem {
	const id = "agents_outdated"
	if !in.AgentsKnown {
		return unknown(id, postureMedium, "nodes could not be read")
	}
	if len(in.OutdatedAgents) == 0 {
		return pass(id, postureMedium)
	}
	return failing(id, postureMedium, slices.Clone(in.OutdatedAgents), linkFix("/nodes"))
}

func anomalyRule(in postureInput) postureItem {
	const id = "login_anomalies"
	if len(in.Anomalies) == 0 {
		return pass(id, postureHigh)
	}
	var subjects []string
	for _, a := range in.Anomalies {
		subjects = append(subjects, a.Kind+" "+a.Subject)
	}
	return failing(id, postureHigh, subjects, linkFix("/settings/audit-log"))
}

// postureScore is 100 minus the weight of every failing item, floored at 0.
func postureScore(items []postureItem) int {
	score := 100
	for _, it := range items {
		if it.Status == postureStatusFail {
			score -= postureWeights[it.Severity]
		}
	}
	return max(score, 0)
}

func postureGrade(score int) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 75:
		return "B"
	case score >= 50:
		return "C"
	}
	return "D"
}
