// Package setupguide derives the Get started checklist from real instance
// state. It is pure: callers gather an Input, Derive returns the guide.
package setupguide

import "strings"

// StepID names one checklist step.
type StepID string

// Step identifiers, in presentation order.
const (
	StepSecureAdmin    StepID = "secure_admin"
	StepSignInRecovery StepID = "sign_in_recovery"
	StepSecurityReview StepID = "security-review"
	StepPublicAddress  StepID = "public_address"
	StepGitProvider    StepID = "git_provider"
	StepFirstApp       StepID = "first_app"
	StepAppDomain      StepID = "app_domain"
	StepDNS            StepID = "dns"
	StepTLS            StepID = "tls"
	StepBackupTarget   StepID = "backup_target"
	StepFirstBackup    StepID = "first_backup"
	StepVerifiedBackup StepID = "verified_restore"
	StepNotifications  StepID = "notifications"
	StepTeammate       StepID = "teammate"
	StepUpgrades       StepID = "upgrades"
)

// State is a step's current state.
type State string

// Step states.
const (
	StateTodo          State = "todo"
	StateInProgress    State = "in_progress"
	StateDone          State = "done"
	StateSkipped       State = "skipped"
	StateNotApplicable State = "not_applicable"
)

// Reason codes explain why a step is not applicable or only partly done.
const (
	ReasonTLSUpstream  = "tls_terminated_upstream"
	ReasonProxyMode    = "proxy_mode"
	ReasonUnavailable  = "unavailable"
	ReasonHTTPOnly     = "http_only"
	ReasonNoRules      = "no_alert_rule"
	ReasonCertsPending = "certificates_pending"
	ReasonACMEOff      = "acme_disabled"
	ReasonManual       = "marked_done"
)

// Override values an operator can record.
const (
	OverrideSkipped = "skipped"
	OverrideDone    = "done"
)

// DomainTLS is one attached app domain and whether it has a healthy cert.
type DomainTLS struct {
	Name        string
	CertHealthy bool
}

// Input is everything Derive needs. Zero values mean "nothing set up yet".
type Input struct {
	SecuredAdmin bool
	// EmailConfigured and SignInApprover say a new browser can be approved
	// without a dashboard that is already open: email can be sent, or a CLI
	// token holds signin:approve.
	EmailConfigured      bool
	SignInApprover       bool
	SecurityReviewed     bool
	DashboardURL         string
	ServerAddress        string
	BehindProxy          bool
	ACMEEnabled          bool
	GitConnected         bool
	Apps                 int
	Domains              []DomainTLS
	DNSProviderConnected bool
	BackupTargets        int
	ControlPlaneBackups  int
	VerifiedBackups      int
	Channels             int
	AlertRules           int
	Users                int
	PendingInvites       int
	AutomaticUpgrades    bool

	// Unavailable lists steps whose source data could not be read.
	Unavailable map[StepID]bool
	// Overrides holds operator overrides keyed by step id.
	Overrides map[StepID]string
	Hidden    bool
}

// Progress counts done items of a multi-item step.
type Progress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// Step is one derived checklist entry.
type Step struct {
	ID              StepID    `json:"id"`
	Title           string    `json:"title"`
	State           State     `json:"state"`
	Optional        bool      `json:"optional"`
	EstimateMinutes int       `json:"estimate_minutes"`
	Href            string    `json:"href"`
	BlockedBy       []StepID  `json:"blocked_by,omitempty"`
	Reason          string    `json:"reason,omitempty"`
	Progress        *Progress `json:"progress,omitempty"`
	Manual          bool      `json:"manual,omitempty"`
}

// Guide is the derived checklist.
type Guide struct {
	Steps         []Step `json:"steps"`
	ReadyPercent  int    `json:"ready_percent"`
	Done          int    `json:"done"`
	Total         int    `json:"total"`
	NextStep      StepID `json:"next_step,omitempty"`
	Complete      bool   `json:"complete"`
	Hidden        bool   `json:"hidden"`
	ServerAddress string `json:"server_address,omitempty"`
}

type stepMeta struct {
	id       StepID
	title    string
	optional bool
	minutes  int
	href     string
	needs    []StepID
}

var catalog = []stepMeta{
	{id: StepSecureAdmin, title: "Secure the admin account", minutes: 2, href: "/settings/security"},
	{id: StepSignInRecovery, title: "Set up a way back in", minutes: 3, href: "/settings/security"},
	{id: StepSecurityReview, title: "Review your security defaults", minutes: 2, href: "/settings/security-runtime"},
	{id: StepPublicAddress, title: "Set the public dashboard address", minutes: 3, href: "/settings/general"},
	{id: StepGitProvider, title: "Connect a Git provider", optional: true, minutes: 3, href: "/settings/github-app"},
	{id: StepFirstApp, title: "Deploy your first app", minutes: 5, href: "/apps"},
	{id: StepAppDomain, title: "Put the app on your domain", minutes: 4, href: "/domains", needs: []StepID{StepFirstApp}},
	{id: StepDNS, title: "Connect a DNS provider or proxy", optional: true, minutes: 3, href: "/domains"},
	{id: StepTLS, title: "Get a valid TLS certificate", minutes: 2, href: "/settings/certificates", needs: []StepID{StepAppDomain}},
	{id: StepBackupTarget, title: "Add a backup target", minutes: 4, href: "/settings/backup-targets"},
	{id: StepFirstBackup, title: "Take the first backup", minutes: 2, href: "/settings/control-plane-backup", needs: []StepID{StepBackupTarget}},
	{id: StepVerifiedBackup, title: "Verify a restore", minutes: 3, href: "/settings/control-plane-backup", needs: []StepID{StepFirstBackup}},
	{id: StepNotifications, title: "Add a notification channel and alert rule", minutes: 4, href: "/settings/notification-channels"},
	{id: StepTeammate, title: "Invite a teammate", optional: true, minutes: 2, href: "/settings/users"},
	{id: StepUpgrades, title: "Choose an upgrade policy", optional: true, minutes: 2, href: "/settings/updates"},
}

// StepIDs returns every step id in presentation order.
func StepIDs() []StepID {
	out := make([]StepID, len(catalog))
	for i, m := range catalog {
		out[i] = m.id
	}
	return out
}

// ValidStep reports whether id names a known step.
func ValidStep(id string) bool {
	for _, m := range catalog {
		if string(m.id) == id {
			return true
		}
	}
	return false
}

type verdict struct {
	state    State
	reason   string
	progress *Progress
}

func derive(id StepID, in Input) verdict {
	switch id {
	case StepSecureAdmin:
		return flag(in.SecuredAdmin)
	case StepSignInRecovery:
		return flag(in.EmailConfigured || in.SignInApprover)
	case StepSecurityReview:
		return flag(in.SecurityReviewed)
	case StepPublicAddress:
		switch {
		case strings.HasPrefix(in.DashboardURL, "https://"):
			return verdict{state: StateDone}
		case in.DashboardURL != "":
			return verdict{state: StateInProgress, reason: ReasonHTTPOnly}
		}
		return verdict{state: StateTodo}
	case StepGitProvider:
		return flag(in.GitConnected)
	case StepFirstApp:
		return flag(in.Apps > 0)
	case StepAppDomain:
		return flag(len(in.Domains) > 0)
	case StepDNS:
		if in.DNSProviderConnected {
			return verdict{state: StateDone}
		}
		if in.BehindProxy {
			return verdict{state: StateDone, reason: ReasonProxyMode}
		}
		return verdict{state: StateTodo}
	case StepTLS:
		return deriveTLS(in)
	case StepBackupTarget:
		return flag(in.BackupTargets > 0)
	case StepFirstBackup:
		return flag(in.ControlPlaneBackups > 0)
	case StepVerifiedBackup:
		return flag(in.VerifiedBackups > 0)
	case StepNotifications:
		switch {
		case in.Channels > 0 && in.AlertRules > 0:
			return verdict{state: StateDone}
		case in.Channels > 0:
			return verdict{state: StateInProgress, reason: ReasonNoRules}
		}
		return verdict{state: StateTodo}
	case StepTeammate:
		return flag(in.Users > 1 || in.PendingInvites > 0)
	case StepUpgrades:
		return flag(in.AutomaticUpgrades)
	}
	return verdict{state: StateTodo}
}

func deriveTLS(in Input) verdict {
	if in.BehindProxy {
		return verdict{state: StateNotApplicable, reason: ReasonTLSUpstream}
	}
	if len(in.Domains) == 0 {
		return verdict{state: StateTodo}
	}
	healthy := 0
	for _, d := range in.Domains {
		if d.CertHealthy {
			healthy++
		}
	}
	p := &Progress{Done: healthy, Total: len(in.Domains)}
	switch {
	case healthy == len(in.Domains):
		return verdict{state: StateDone, progress: p}
	case !in.ACMEEnabled:
		return verdict{state: StateTodo, reason: ReasonACMEOff, progress: p}
	}
	return verdict{state: StateInProgress, reason: ReasonCertsPending, progress: p}
}

func flag(done bool) verdict {
	if done {
		return verdict{state: StateDone}
	}
	return verdict{state: StateTodo}
}

func resolved(s State) bool {
	return s == StateDone || s == StateSkipped || s == StateNotApplicable
}

// Derive computes the guide. Derived "done" always wins over an override, so
// a skipped step that later completes shows as done.
func Derive(in Input) Guide {
	steps := make([]Step, 0, len(catalog))
	byID := make(map[StepID]State, len(catalog))
	for _, m := range catalog {
		v := derive(m.id, in)
		step := Step{
			ID: m.id, Title: m.title, Optional: m.optional,
			EstimateMinutes: m.minutes, Href: m.href,
			State: v.state, Reason: v.reason, Progress: v.progress,
		}
		if in.Unavailable[m.id] {
			step.State, step.Reason, step.Progress = StateNotApplicable, ReasonUnavailable, nil
		}
		if step.State != StateDone && step.State != StateNotApplicable {
			switch in.Overrides[m.id] {
			case OverrideDone:
				step.State, step.Reason, step.Manual = StateDone, ReasonManual, true
			case OverrideSkipped:
				step.State = StateSkipped
			}
		}
		byID[m.id] = step.State
		steps = append(steps, step)
	}
	for i, m := range catalog {
		for _, dep := range m.needs {
			if byID[dep] != StateDone && byID[dep] != StateNotApplicable {
				steps[i].BlockedBy = append(steps[i].BlockedBy, dep)
			}
		}
	}
	return summarize(steps, in)
}

func summarize(steps []Step, in Input) Guide {
	g := Guide{Steps: steps, Hidden: in.Hidden, ServerAddress: in.ServerAddress, Complete: true}
	reqDone, reqTotal := 0, 0
	for _, s := range steps {
		if s.State == StateNotApplicable {
			continue
		}
		g.Total++
		if s.State == StateDone {
			g.Done++
		}
		if !s.Optional {
			reqTotal++
			if s.State == StateDone {
				reqDone++
			}
		}
		if !resolved(s.State) {
			g.Complete = false
		}
	}
	if reqTotal > 0 {
		g.ReadyPercent = reqDone * 100 / reqTotal
	} else {
		g.ReadyPercent = 100
	}
	g.NextStep = nextStep(steps)
	return g
}

// nextStep prefers an unblocked required step, then an unblocked optional one.
func nextStep(steps []Step) StepID {
	for _, wantOptional := range []bool{false, true} {
		for _, s := range steps {
			if s.Optional != wantOptional || resolved(s.State) || len(s.BlockedBy) > 0 {
				continue
			}
			return s.ID
		}
	}
	return ""
}
