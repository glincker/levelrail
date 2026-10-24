package setupguide

import "testing"

func stateOf(t *testing.T, g Guide, id StepID) Step {
	t.Helper()
	for _, s := range g.Steps {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("step %q missing", id)
	return Step{}
}

func TestDeriveStepStates(t *testing.T) {
	tests := []struct {
		name       string
		in         Input
		step       StepID
		want       State
		wantReason string
	}{
		{"fresh admin is todo", Input{}, StepSecureAdmin, StateTodo, ""},
		{"secured admin", Input{SecuredAdmin: true}, StepSecureAdmin, StateDone, ""},
		{"no dashboard url", Input{}, StepPublicAddress, StateTodo, ""},
		{"http dashboard url is in progress", Input{DashboardURL: "http://203.0.113.7:8080"}, StepPublicAddress, StateInProgress, ReasonHTTPOnly},
		{"https dashboard url", Input{DashboardURL: "https://dash.example.com"}, StepPublicAddress, StateDone, ""},
		{"git connected", Input{GitConnected: true}, StepGitProvider, StateDone, ""},
		{"no apps", Input{}, StepFirstApp, StateTodo, ""},
		{"one app", Input{Apps: 1}, StepFirstApp, StateDone, ""},
		{"domain attached", Input{Domains: []DomainTLS{{Name: "a.example.com"}}}, StepAppDomain, StateDone, ""},
		{"dns provider", Input{DNSProviderConnected: true}, StepDNS, StateDone, ""},
		{"dns satisfied by proxy mode", Input{BehindProxy: true}, StepDNS, StateDone, ReasonProxyMode},
		{"dns todo", Input{}, StepDNS, StateTodo, ""},
		{"tls not applicable behind proxy", Input{BehindProxy: true}, StepTLS, StateNotApplicable, ReasonTLSUpstream},
		{"tls todo without domains", Input{ACMEEnabled: true}, StepTLS, StateTodo, ""},
		{"tls acme off", Input{Domains: []DomainTLS{{Name: "a.example.com"}}}, StepTLS, StateTodo, ReasonACMEOff},
		{"tls pending", Input{ACMEEnabled: true, Domains: []DomainTLS{{Name: "a.example.com"}, {Name: "b.example.com", CertHealthy: true}}}, StepTLS, StateInProgress, ReasonCertsPending},
		{"tls healthy", Input{ACMEEnabled: true, Domains: []DomainTLS{{Name: "a.example.com", CertHealthy: true}}}, StepTLS, StateDone, ""},
		{"backup target", Input{BackupTargets: 1}, StepBackupTarget, StateDone, ""},
		{"first backup", Input{ControlPlaneBackups: 2}, StepFirstBackup, StateDone, ""},
		{"verified restore", Input{VerifiedBackups: 1}, StepVerifiedBackup, StateDone, ""},
		{"channel without rule", Input{Channels: 1}, StepNotifications, StateInProgress, ReasonNoRules},
		{"channel and rule", Input{Channels: 1, AlertRules: 1}, StepNotifications, StateDone, ""},
		{"pending invite counts", Input{Users: 1, PendingInvites: 1}, StepTeammate, StateDone, ""},
		{"second user counts", Input{Users: 2}, StepTeammate, StateDone, ""},
		{"solo admin", Input{Users: 1}, StepTeammate, StateTodo, ""},
		{"upgrades auto", Input{AutomaticUpgrades: true}, StepUpgrades, StateDone, ""},
		{"unavailable source", Input{Unavailable: map[StepID]bool{StepFirstBackup: true}}, StepFirstBackup, StateNotApplicable, ReasonUnavailable},
		{"skip override", Input{Overrides: map[StepID]string{StepTeammate: OverrideSkipped}}, StepTeammate, StateSkipped, ""},
		{"done override", Input{Overrides: map[StepID]string{StepTeammate: OverrideDone}}, StepTeammate, StateDone, ReasonManual},
		{"derived done beats skip", Input{Apps: 1, Overrides: map[StepID]string{StepFirstApp: OverrideSkipped}}, StepFirstApp, StateDone, ""},
		{"override cannot revive n/a", Input{BehindProxy: true, Overrides: map[StepID]string{StepTLS: OverrideDone}}, StepTLS, StateNotApplicable, ReasonTLSUpstream},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := stateOf(t, Derive(tt.in), tt.step)
			if s.State != tt.want || s.Reason != tt.wantReason {
				t.Fatalf("%s = (%s, %q), want (%s, %q)", tt.step, s.State, s.Reason, tt.want, tt.wantReason)
			}
		})
	}
}

func TestDeriveBlockingAndNext(t *testing.T) {
	g := Derive(Input{})
	if g.NextStep != StepSecureAdmin {
		t.Fatalf("fresh next = %q, want %q", g.NextStep, StepSecureAdmin)
	}
	if b := stateOf(t, g, StepAppDomain).BlockedBy; len(b) != 1 || b[0] != StepFirstApp {
		t.Fatalf("app_domain blocked_by = %v, want [first_app]", b)
	}
	if b := stateOf(t, g, StepVerifiedBackup).BlockedBy; len(b) != 1 || b[0] != StepFirstBackup {
		t.Fatalf("verified_restore blocked_by = %v, want [first_backup]", b)
	}

	g = Derive(Input{
		SecuredAdmin: true, SecurityReviewed: true, EmailConfigured: true, DashboardURL: "https://d.example.com", Apps: 1,
		Domains: []DomainTLS{{Name: "a.example.com", CertHealthy: true}}, ACMEEnabled: true,
		BackupTargets: 1, ControlPlaneBackups: 1,
	})
	if g.NextStep != StepVerifiedBackup {
		t.Fatalf("next = %q, want %q", g.NextStep, StepVerifiedBackup)
	}
	if len(stateOf(t, g, StepAppDomain).BlockedBy) != 0 {
		t.Fatal("app_domain should be unblocked once the app exists")
	}
}

func TestDeriveReadyPercent(t *testing.T) {
	tests := []struct {
		name         string
		in           Input
		wantPercent  int
		wantComplete bool
	}{
		{"fresh", Input{}, 0, false},
		{"proxy mode drops tls from the denominator", Input{BehindProxy: true, SecuredAdmin: true}, 10, false},
		{"all required done, optional todo", Input{
			SecuredAdmin: true, SecurityReviewed: true, EmailConfigured: true, DashboardURL: "https://d.example.com", Apps: 1, ACMEEnabled: true,
			Domains:       []DomainTLS{{Name: "a.example.com", CertHealthy: true}},
			BackupTargets: 1, ControlPlaneBackups: 1, VerifiedBackups: 1, Channels: 1, AlertRules: 1,
		}, 100, false},
		{"everything resolved", Input{
			SecuredAdmin: true, SecurityReviewed: true, SignInApprover: true, DashboardURL: "https://d.example.com", Apps: 1, ACMEEnabled: true,
			Domains:       []DomainTLS{{Name: "a.example.com", CertHealthy: true}},
			BackupTargets: 1, ControlPlaneBackups: 1, VerifiedBackups: 1, Channels: 1, AlertRules: 1,
			GitConnected: true, DNSProviderConnected: true, Users: 2, AutomaticUpgrades: true,
		}, 100, true},
		{"skipped required step does not count as ready", Input{
			SecuredAdmin: true, Overrides: map[StepID]string{StepFirstApp: OverrideSkipped},
		}, 9, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := Derive(tt.in)
			if g.ReadyPercent != tt.wantPercent || g.Complete != tt.wantComplete {
				t.Fatalf("percent=%d complete=%v, want %d/%v", g.ReadyPercent, g.Complete, tt.wantPercent, tt.wantComplete)
			}
		})
	}
}

func TestValidStepCoversCatalog(t *testing.T) {
	for _, id := range StepIDs() {
		if !ValidStep(string(id)) {
			t.Fatalf("ValidStep(%q) = false", id)
		}
	}
	if ValidStep("nope") {
		t.Fatal("ValidStep(nope) = true")
	}
}

func TestDeriveSignInRecovery(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want State
	}{
		{"nothing set up", Input{}, StateTodo},
		{"email can be sent", Input{EmailConfigured: true}, StateDone},
		{"a token can approve", Input{SignInApprover: true}, StateDone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stateOf(t, Derive(tt.in), StepSignInRecovery).State; got != tt.want {
				t.Fatalf("state = %s, want %s", got, tt.want)
			}
		})
	}
}
