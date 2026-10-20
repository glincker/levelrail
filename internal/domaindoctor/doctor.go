// Package domaindoctor runs an ordered set of read-only checks against one
// domain (DNS, CAA, ports, TLS, HTTP, redirects, HSTS, platform state) and
// turns each result into a fix an operator can act on.
package domaindoctor

import (
	"context"
	"net"
	"slices"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/fixit"
)

// State is the outcome of one check.
type State string

// The five check outcomes.
const (
	StatePass        State = "pass"
	StateWarn        State = "warn"
	StateFail        State = "fail"
	StateSkipped     State = "skipped"
	StateUnavailable State = "unavailable"
)

// Overall report statuses.
const (
	StatusHealthy  = "healthy"
	StatusWarnings = "warnings"
	StatusProblems = "problems"
)

// Tiers order fixes: traffic blockers first, then certificate blockers,
// then things that break soon, then hygiene.
const (
	TierTraffic     = 1
	TierCertificate = 2
	TierSoon        = 3
	TierHygiene     = 4
)

// Action kinds a fix can carry.
const (
	ActionRequest = "request"
	ActionLink    = "link"
	ActionCopy    = "copy"
)

// Action is the one primary thing a fix offers: an API request to run
// ("POST /api/v1/..." with an optional JSON Body), a page to open, or a
// Value to copy.
type Action struct {
	Kind  string            `json:"kind"`
	Label string            `json:"label"`
	API   string            `json:"api,omitempty"`
	Value string            `json:"value,omitempty"`
	Body  map[string]string `json:"body,omitempty"`
}

// Record is one DNS record an operator or the connected DNS provider creates.
type Record struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// Fix is the remedy for a failed or warning check. Kind says how much the
// control plane can do: auto applies Action itself, guided shows exact
// Records or values, manual only explains. Every kind is followed by a
// re-check of the whole domain.
type Fix struct {
	Summary string     `json:"summary"`
	Kind    fixit.Kind `json:"kind"`
	Action  *Action    `json:"action,omitempty"`
	Records []Record   `json:"records,omitempty"`
	// CLI is the secondary, collapsed way to do the same from a terminal.
	CLI string `json:"cli,omitempty"`
}

// Check is one ordered result.
type Check struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	State  State  `json:"state"`
	Tier   int    `json:"tier"`
	Detail string `json:"detail,omitempty"`
	Fix    *Fix   `json:"fix,omitempty"`
}

// Report is the doctor's full answer for one domain.
type Report struct {
	Domain    string    `json:"domain"`
	App       string    `json:"app,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
	Status    string    `json:"status"`
	// Probed is false when the doctor refused to connect to the domain
	// (it does not resolve to this server), with ProbeNote saying why.
	Probed    bool    `json:"probed"`
	ProbeNote string  `json:"probe_note,omitempty"`
	Checks    []Check `json:"checks"`
	// Records are the DNS records this domain needs, inferred from apex versus
	// subdomain and this server's address.
	Records     []Record     `json:"records,omitempty"`
	Propagation *Propagation `json:"propagation,omitempty"`
	// GoLive is the dashboard path of the guided checklist for this app and domain.
	GoLive string `json:"go_live,omitempty"`
}

// Propagation states.
const (
	PropagationSettled     = "settled"
	PropagationPropagating = "propagating"
	PropagationWaiting     = "waiting"
)

// Propagation tells the dashboard whether DNS has caught up and when to
// look again.
type Propagation struct {
	State            string    `json:"state"`
	Detail           string    `json:"detail"`
	NextCheckSeconds int       `json:"next_check_seconds,omitempty"`
	NextCheckAt      time.Time `json:"next_check_at,omitzero"`
}

// PortHolder is a container publishing host port 80 or 443.
type PortHolder struct {
	Port      int
	Container string
	Kind      string
}

// AppFacts is the platform's view of the app serving the domain.
type AppFacts struct {
	Known   bool
	Running bool
	Ready   bool
	Detail  string
}

// CertFacts is the stored certificate for the domain.
type CertFacts struct {
	Issuer   string
	NotAfter time.Time
	Status   string
	Renewal  string
	Source   string
}

// ACMEFailureFacts is the CA's last recorded error for the domain.
type ACMEFailureFacts struct {
	Error   string
	Action  string
	Renewal bool
}

// Facts is everything the API layer already knows from stored state.
type Facts struct {
	Domain    string
	App       string
	ServerIPs []string
	// HTTPPort and HTTPSPort are the ports clients use; 0 means 80 and 443.
	HTTPPort              int
	HTTPSPort             int
	TLSTerminatedUpstream bool
	ACMEEnabled           bool
	AppState              AppFacts
	HeldBack              bool
	PortHoldersKnown      bool
	PortHolders           []PortHolder
	Cert                  *CertFacts
	ACMEFailure           *ACMEFailureFacts
	// Target is set for a standalone domain, replacing the app check.
	Target *TargetFacts
	// ServerHost is the configured public hostname, used as a CNAME target.
	ServerHost string
	// DNSProvider and DNSZone describe the connected DNS provider that can
	// create records; empty means none.
	DNSProvider string
	DNSZone     string
	// CLIName is the command line tool's name for the secondary CLI hint.
	CLIName string
}

// TargetFacts is a standalone domain's proxy target and whether this server
// could connect to it just now.
type TargetFacts struct {
	Display   string
	Reachable bool
	Detail    string
}

// Options bound the run.
type Options struct {
	Timeout        time.Duration
	ProbeTimeout   time.Duration
	ExpiryWarnDays int
	MaxRedirects   int
	Now            func() time.Time
}

// Probes are the network seams; tests replace each one.
type Probes struct {
	DNS  DNSProbe
	TLS  TLSProbe
	HTTP HTTPProbe
}

const (
	defaultTimeout        = 20 * time.Second
	defaultProbeTimeout   = 4 * time.Second
	defaultExpiryWarnDays = 30
	defaultMaxRedirects   = 6
)

func (o Options) withDefaults() Options {
	if o.Timeout <= 0 {
		o.Timeout = defaultTimeout
	}
	if o.ProbeTimeout <= 0 {
		o.ProbeTimeout = defaultProbeTimeout
	}
	if o.ExpiryWarnDays <= 0 {
		o.ExpiryWarnDays = defaultExpiryWarnDays
	}
	if o.MaxRedirects <= 0 {
		o.MaxRedirects = defaultMaxRedirects
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return o
}

// run carries one doctor run's shared state between checks.
type run struct {
	facts  Facts
	probes Probes
	opts   Options
	report *Report

	answers  []ResolverAnswer
	resolved []string
	probeIP  string
	tls      *PeerCert
	tlsErr   error
	https    *HTTPResult
}

func overall(checks []Check) string {
	status := StatusHealthy
	for _, c := range checks {
		switch c.State {
		case StateFail:
			return StatusProblems
		case StateWarn:
			status = StatusWarnings
		}
	}
	return status
}

// Prioritized returns the failing and warning checks, fails before warns,
// lower tier first, stable within a tier.
func Prioritized(checks []Check) []Check {
	var out []Check
	for _, c := range checks {
		if c.State == StateFail || c.State == StateWarn {
			out = append(out, c)
		}
	}
	slices.SortStableFunc(out, func(a, b Check) int {
		if a.State != b.State {
			if a.State == StateFail {
				return -1
			}
			return 1
		}
		return a.Tier - b.Tier
	})
	return out
}

func (rn *run) probeCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, rn.opts.ProbeTimeout)
}

func (rn *run) httpsPort() int {
	if rn.facts.HTTPSPort > 0 {
		return rn.facts.HTTPSPort
	}
	return 443
}

func (rn *run) httpPort() int {
	if rn.facts.HTTPPort > 0 {
		return rn.facts.HTTPPort
	}
	return 80
}

func (rn *run) appDomainAPI(suffix string) string {
	if rn.facts.App == "" {
		return ""
	}
	return "/api/v1/apps/" + rn.facts.App + "/domains/" + rn.facts.Domain + suffix
}

func joinHostPort(ip string, port int) string {
	return net.JoinHostPort(ip, strconv.Itoa(port))
}
