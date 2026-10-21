// Package golive turns the platform's separate checks (app readiness, ports,
// host firewall, DNS, certificate, end-to-end HTTPS) into one ordered
// checklist for an app and a domain, each step with a status, evidence and one
// primary action. It does no I/O of its own except SelfCheck.
package golive

import (
	"time"

	"github.com/GLINCKER/levelrail/internal/domaindoctor"
	"github.com/GLINCKER/levelrail/internal/fixit"
	"github.com/GLINCKER/levelrail/internal/hostfw"
	"github.com/GLINCKER/levelrail/internal/hostprovider"
)

// Step ids, in checklist order.
const (
	StepApp         = "app"
	StepPort        = "port"
	StepFirewall    = "firewall"
	StepDNS         = "dns"
	StepCertificate = "certificate"
	StepLive        = "live"
)

// Status of one step.
type Status string

// Status values.
const (
	StatusPending Status = "pending"
	StatusOK      Status = "ok"
	StatusWarn    Status = "warn"
	StatusFail    Status = "fail"
	StatusUnknown Status = "unknown"
)

// State is the one-word summary used by badges.
type State string

// State values.
const (
	StateNotLive            State = "not_live"
	StateWaitingDNS         State = "waiting_dns"
	StateWaitingCertificate State = "waiting_certificate"
	StateLive               State = "live"
	StateNeedsAttention     State = "needs_attention"
)

// Action kinds.
const (
	ActionFix     = "fix"
	ActionRequest = "request"
	ActionRecheck = "recheck"
	ActionLink    = "link"
	ActionCopy    = "copy"
)

// Action is the one primary thing a step offers.
type Action struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	// FixID names a typed fix applied through the fixit endpoints.
	FixID string `json:"fix_id,omitempty"`
	// API is "METHOD /path" for a request action.
	API   string         `json:"api,omitempty"`
	Body  map[string]any `json:"body,omitempty"`
	Href  string         `json:"href,omitempty"`
	Value string         `json:"value,omitempty"`
	// Blocked, when set, is why the action cannot run yet.
	Blocked string `json:"blocked,omitempty"`
}

// Evidence is one fact behind a step status.
type Evidence struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Copy  bool   `json:"copy,omitempty"`
}

// FixPreview is what an automatic fix would do, shown before it runs.
type FixPreview struct {
	FixID      string   `json:"fix_id"`
	Kind       string   `json:"kind"`
	Commands   []string `json:"commands"`
	Undo       []string `json:"undo"`
	Persistent bool     `json:"persistent"`
	Note       string   `json:"note,omitempty"`
	Refusal    string   `json:"refusal,omitempty"`
}

// Step is one checklist row.
type Step struct {
	ID       string     `json:"id"`
	Title    string     `json:"title"`
	Status   Status     `json:"status"`
	Summary  string     `json:"summary"`
	Evidence []Evidence `json:"evidence"`
	Primary  *Action    `json:"primary,omitempty"`
	// CLI is the secondary, collapsed way to do the same from a terminal.
	CLI         string     `json:"cli,omitempty"`
	NextCheckAt *time.Time `json:"next_check_at,omitempty"`
	DocsURL     string     `json:"docs_url,omitempty"`

	Records     []domaindoctor.Record  `json:"records,omitempty"`
	FirewallFix *FixPreview            `json:"firewall_fix,omitempty"`
	Provider    *hostprovider.Guidance `json:"provider,omitempty"`
	// Scope says where a probe ran, so a control plane check is never read as proof from the internet.
	Scope string `json:"scope,omitempty"`
}

// Checklist is the whole guided flow for one app and domain.
type Checklist struct {
	App       string          `json:"app"`
	Domain    string          `json:"domain"`
	State     State           `json:"state"`
	Live      bool            `json:"live"`
	Next      string          `json:"next,omitempty"`
	URL       string          `json:"url,omitempty"`
	Steps     []Step          `json:"steps"`
	Timeline  []TimelineEvent `json:"timeline"`
	CheckedAt time.Time       `json:"checked_at"`
}

// AppInput is the readiness of the app.
type AppInput struct {
	Known         bool
	Running       bool
	Ready         bool
	Detail        string
	DiagnosisHref string
}

// PortProbe is a loopback request to the container's published port.
type PortProbe struct {
	OK      bool
	Status  int
	Failure string
}

// PortInput is how the app is reachable before any domain exists.
type PortInput struct {
	ZeroConfigURL string
	DirectURL     string
	// DirectAllowed is true when the app exposure feature can publish a direct port.
	DirectAllowed bool
	Probe         *PortProbe
	NetworkHref   string
}

// FirewallInput is the host firewall state and the typed fix for it.
type FirewallInput struct {
	Host     hostfw.Status
	Plan     hostfw.Plan
	FixID    string
	FixKind  fixit.Kind
	Provider *hostprovider.Guidance
}

// CertStatus is the certificate progress for the domain.
type CertStatus struct {
	State       string
	Staging     bool
	Trusted     bool
	Issuer      string
	NotBefore   time.Time
	NotAfter    time.Time
	RenewalAt   time.Time
	Error       string
	Explanation string
	Hint        string
	NextSafeAt  *time.Time
}

// CertInput is what the certificate step decides from.
type CertInput struct {
	Ready      bool
	Blocker    string
	BlockerFix string
	NextSafeAt *time.Time
	Status     CertStatus
	CertAPI    string
}

// Input is everything Build decides from.
type Input struct {
	App       string
	Domain    string
	Now       time.Time
	PollEvery time.Duration
	AppState  AppInput
	Port      PortInput
	Firewall  FirewallInput
	Report    *domaindoctor.Report
	Cert      CertInput
	Live      *SelfCheckResult
	Seen      *Seen
	// ChallengeStartedAt is when the certificate request began, zero when none.
	ChallengeStartedAt time.Time
	// Failure is the CA's last recorded error for the domain.
	Failure *Failure
	CLIName string
}

// Failure is one recorded certificate error.
type Failure struct {
	At          time.Time
	Error       string
	Explanation string
	Renewal     bool
}
