package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

var postureNow = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

// healthyPosture passes every rule; each case breaks exactly one fact.
func healthyPosture() postureInput {
	yes, no, zero := true, false, 0
	rotated := postureNow.Add(-24 * time.Hour)
	expires := postureNow.Add(30 * 24 * time.Hour)
	used := postureNow.Add(-time.Hour)
	return postureInput{
		Now:    postureNow,
		Policy: securityPolicy{ApprovalScope: approvalScopeAllMethods, WarnUnusedDays: 30},
		Users: []postureUser{
			{ID: "u1", Name: "admin@example.com", Admin: true, TOTP: true, RecoveryCodes: 8, Passkeys: 1},
			{ID: "u2", Name: "dev@example.com", Passkeys: 0},
		},
		UsersKnown:        true,
		Tokens:            []store.APIToken{{ID: "t1", Name: "ci", Abilities: []string{AbilityRead}, CreatedAt: postureNow.Add(-48 * time.Hour), LastUsedAt: &used, ExpiresAt: &expires}},
		TokensKnown:       true,
		Sessions:          []postureSession{{UserID: "u1", CreatedAt: postureNow.Add(-time.Hour)}},
		SessionsKnown:     true,
		SessionMaxAge:     7 * 24 * time.Hour,
		NewDeviceApproval: true,
		CodeLoginAdmins:   &no,
		Exposure:          &postureExposure{},
		OffBoxBackups:     &yes,
		MasterKey:         true,
		MasterKeyKnown:    true,
		MasterKeyRotated:  &rotated,
		MasterKeyMaxAge:   365 * 24 * time.Hour,
		StaleSecrets:      &zero,
		SecretMaxAgeDays:  90,
		TLSKnown:          true,
		ACMEEnabled:       true,
		RealCert:          true,
		HSTS:              true,
		AgentsKnown:       true,
	}
}

func postureByID(items []postureItem) map[string]postureItem {
	out := map[string]postureItem{}
	for _, it := range items {
		out[it.ID] = it
	}
	return out
}

func TestEvaluatePosture_HealthyPassesEveryRule(t *testing.T) {
	items := evaluatePosture(healthyPosture())
	for _, it := range items {
		if it.Status != postureStatusPass {
			t.Errorf("%s = %s, want pass", it.ID, it.Status)
		}
	}
	if got := postureScore(items); got != 100 {
		t.Fatalf("score = %d, want 100", got)
	}
}

func TestEvaluatePosture_EachRuleFails(t *testing.T) {
	old := postureNow.Add(-400 * 24 * time.Hour)
	stale, yes, no := 3, true, false
	tests := []struct {
		id     string
		status string
		mutate func(*postureInput)
	}{
		{"admin_without_mfa", postureStatusFail, func(in *postureInput) { in.Users[0].TOTP, in.Users[0].Passkeys = false, 0 }},
		{"account_flagged", postureStatusFail, func(in *postureInput) { in.Users[1].Flagged = true }},
		{"recovery_codes_missing", postureStatusFail, func(in *postureInput) { in.Users[0].RecoveryCodes = 0 }},
		{"no_admin_passkey", postureStatusFail, func(in *postureInput) { in.Users[0].Passkeys = 0 }},
		{"token_root", postureStatusFail, func(in *postureInput) { in.Tokens[0].Abilities = []string{AbilityRoot} }},
		{"token_no_expiry", postureStatusFail, func(in *postureInput) { in.Tokens[0].ExpiresAt = nil }},
		{"token_signin_approve", postureStatusFail, func(in *postureInput) { in.Tokens[0].Abilities = []string{AbilitySignInApprove} }},
		{"token_unused", postureStatusFail, func(in *postureInput) { in.Tokens[0].LastUsedAt = &old; in.Tokens[0].CreatedAt = old }},
		{"sessions_old", postureStatusFail, func(in *postureInput) { in.Sessions[0].CreatedAt = old }},
		{"new_device_approval_off", postureStatusFail, func(in *postureInput) { in.NewDeviceApproval = false }},
		{"approval_password_only", postureStatusFail, func(in *postureInput) { in.Policy.ApprovalScope = approvalScopePasswordOnly }},
		{"code_login_admins", postureStatusFail, func(in *postureInput) { in.CodeLoginAdmins = &yes }},
		{"public_exposure", postureStatusFail, func(in *postureInput) { in.Exposure.Attention = 2 }},
		{"docker_api_exposed", postureStatusFail, func(in *postureInput) { in.Exposure.DockerAPI = 1 }},
		{"offbox_backup_off", postureStatusFail, func(in *postureInput) { in.OffBoxBackups = &no }},
		{"master_key_rotation", postureStatusFail, func(in *postureInput) { in.MasterKeyRotated = nil }},
		{"master_key_rotation", postureStatusFail, func(in *postureInput) { in.MasterKey = false }},
		{"secrets_old", postureStatusFail, func(in *postureInput) { in.StaleSecrets = &stale }},
		{"tls_not_public", postureStatusFail, func(in *postureInput) { in.InsecureDomains = []string{"app.example.com"} }},
		{"hsts_off", postureStatusFail, func(in *postureInput) { in.HSTS = false }},
		{"agents_outdated", postureStatusFail, func(in *postureInput) { in.OutdatedAgents = []string{"node-2"} }},
		{"login_anomalies", postureStatusFail, func(in *postureInput) {
			in.Anomalies = []loginAnomaly{{Kind: anomalyKindIP, Subject: "203.0.113.9", Count: 20, At: postureNow}}
		}},
		{"admin_without_mfa", postureStatusUnknown, func(in *postureInput) { in.UsersKnown = false }},
		{"token_root", postureStatusUnknown, func(in *postureInput) { in.TokensKnown = false }},
		{"public_exposure", postureStatusUnknown, func(in *postureInput) { in.Exposure = nil }},
		{"offbox_backup_off", postureStatusUnknown, func(in *postureInput) { in.OffBoxBackups = nil }},
		{"tls_not_public", postureStatusUnknown, func(in *postureInput) { in.TLSKnown = false }},
		{"agents_outdated", postureStatusUnknown, func(in *postureInput) { in.AgentsKnown = false }},
		{"tls_not_public", postureStatusPass, func(in *postureInput) {
			in.InsecureDomains, in.TLSUpstream = []string{"app.example.com"}, true
		}},
		{"hsts_off", postureStatusPass, func(in *postureInput) { in.HSTS, in.RealCert = false, false }},
	}
	for _, tt := range tests {
		t.Run(tt.id+"/"+tt.status, func(t *testing.T) {
			in := healthyPosture()
			in.Exposure = &postureExposure{}
			in.Users = append([]postureUser(nil), in.Users...)
			in.Tokens = append([]store.APIToken(nil), in.Tokens...)
			in.Sessions = append([]postureSession(nil), in.Sessions...)
			tt.mutate(&in)
			items := evaluatePosture(in)
			got := postureByID(items)[tt.id]
			if got.Status != tt.status {
				t.Fatalf("%s = %+v, want %s", tt.id, got, tt.status)
			}
			if tt.status == postureStatusFail {
				if got.Fix == nil {
					t.Fatalf("%s fails without a fix", tt.id)
				}
				if items[0].Status != postureStatusFail {
					t.Fatal("failing items must sort first")
				}
				if postureScore(items) >= 100 {
					t.Fatal("a failing item must lower the score")
				}
			}
		})
	}
}

func TestPostureScoreAndGrade(t *testing.T) {
	tests := []struct {
		name  string
		items []postureItem
		score int
		grade string
	}{
		{"nothing failing", []postureItem{{Severity: postureCritical, Status: postureStatusPass}}, 100, "A"},
		{"unknown costs nothing", []postureItem{{Severity: postureCritical, Status: postureStatusUnknown}}, 100, "A"},
		{"one critical", []postureItem{{Severity: postureCritical, Status: postureStatusFail}}, 75, "B"},
		{"floors at zero", []postureItem{
			{Severity: postureCritical, Status: postureStatusFail}, {Severity: postureCritical, Status: postureStatusFail},
			{Severity: postureCritical, Status: postureStatusFail}, {Severity: postureCritical, Status: postureStatusFail},
			{Severity: postureHigh, Status: postureStatusFail},
		}, 0, "D"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := postureScore(tt.items); got != tt.score || postureGrade(got) != tt.grade {
				t.Fatalf("score %d grade %s, want %d %s", got, postureGrade(got), tt.score, tt.grade)
			}
		})
	}
}
