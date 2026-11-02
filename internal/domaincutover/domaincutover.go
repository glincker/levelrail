// Package domaincutover turns the facts about a domain moving to this server into
// an ordered checklist with a downtime verdict. It does no I/O: the API layer
// gathers the facts (probes, DNS, certificate state) and calls Evaluate.
package domaincutover

import (
	"fmt"
	"strings"
	"time"
)

// Step ids, in checklist order.
const (
	StepApp           = "app"
	StepCertificate   = "certificate"
	StepCurrentDNS    = "current_dns"
	StepSwitch        = "switch_dns"
	StepAuthoritative = "authoritative"
	StepLive          = "live"
)

// State of one step.
type State string

// State values.
const (
	StatePass State = "pass"
	StateWait State = "wait"
	StateFail State = "fail"
	StateSkip State = "skip"
)

// Risk is the downtime verdict.
type Risk string

// Risk values.
const (
	RiskNone    Risk = "none"
	RiskLikely  Risk = "likely"
	RiskUnknown Risk = "unknown"
)

// Where current DNS points.
const (
	PointsNone       = "none"
	PointsThis       = "this_server"
	PointsElsewhere  = "previous_server"
	PointsCloudflare = "cloudflare_proxy"
)

// Certificate progress, matching the guided certificate flow.
const (
	CertOff     = "off"
	CertPending = "pending"
	CertIssued  = "issued"
	CertFailed  = "failed"
)

// DNS check statuses the API layer already produces.
const (
	DNSConnected    = "connected"
	DNSPropagating  = "propagating"
	DNSNotResolving = "not_resolving"
	DNSElsewhere    = "resolves_elsewhere"
)

// Docs links used by the checklist.
const (
	RunbookHref    = "/acme-verification-runbook"
	CloudflareDocs = "/cdn-and-caching"
	GuideHref      = "/move-a-domain-with-no-downtime"
)

// Probe is one HTTPS probe outcome.
type Probe struct {
	// Done is false when the probe was not run (and why is in Err).
	Done        bool
	Status      int
	CertTrusted bool
	CertIssuer  string
	Err         string
}

// Answered is true when the app (not an error page of the proxy) replied.
func (p Probe) Answered() bool { return p.Done && p.Err == "" && p.Status > 0 && p.Status < 500 }

// Facts is everything Evaluate needs.
type Facts struct {
	Domain string
	// App is the pre-flight probe against this server's own ingress.
	App Probe
	// UpstreamTLS is true when a proxy in front owns certificates.
	UpstreamTLS bool

	CertState         string
	CertIssuer        string
	CertError         string
	CertExplanation   string
	ProviderConnected bool

	DNSStatus    string
	Resolved     []string
	ExpectedIPv4 []string
	ExpectedIPv6 []string
	ExpectedHost string
	// Proxied is true when every resolved address is a Cloudflare proxy address.
	Proxied bool
	// TTLSeconds is the live record's TTL, 0 when unknown.
	TTLSeconds int
	// AuthKnown is true when the authoritative servers were asked.
	AuthKnown          bool
	AuthTotal, AuthHit int
	// Live is the end to end probe over the real name.
	Live Probe
}

// Options are the tunables, read from env by the caller.
type Options struct {
	// LowTTL is the TTL at or below which a cutover is quick, and the value advised.
	LowTTL int
	// DefaultETA is the propagation estimate when the TTL is unknown, in seconds.
	DefaultETA int
	// CertETA is the usual time a DNS-01 certificate takes, in seconds.
	CertETA int
}

// Action is the one thing a step offers.
type Action struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	API   string `json:"api,omitempty"`
	Href  string `json:"href,omitempty"`
	CLI   string `json:"cli,omitempty"`
}

// Step is one checklist row.
type Step struct {
	ID         string  `json:"id"`
	State      State   `json:"state"`
	Title      string  `json:"title"`
	Detail     string  `json:"detail"`
	ETASeconds int     `json:"eta_seconds,omitempty"`
	Action     *Action `json:"action,omitempty"`
}

// Record is the exact DNS record to create or change.
type Record struct {
	Type       string `json:"type"`
	Name       string `json:"name"`
	Value      string `json:"value"`
	TTLSeconds int    `json:"ttl_seconds"`
}

// DNSView describes where the name points now.
type DNSView struct {
	Points     string   `json:"points"`
	Status     string   `json:"status"`
	Resolved   []string `json:"resolved,omitempty"`
	TTLSeconds int      `json:"ttl_seconds,omitempty"`
	LowerTTLTo int      `json:"lower_ttl_to"`
	Records    []Record `json:"records,omitempty"`
}

// CertView describes the certificate step for the dashboard action.
type CertView struct {
	State             string `json:"state"`
	Issuer            string `json:"issuer,omitempty"`
	Error             string `json:"error,omitempty"`
	Explanation       string `json:"explanation,omitempty"`
	ProviderConnected bool   `json:"provider_connected"`
	RunbookHref       string `json:"runbook_href"`
}

// Result is the full checklist.
type Result struct {
	Domain        string    `json:"domain"`
	Risk          Risk      `json:"risk"`
	ReadyToSwitch bool      `json:"ready_to_switch"`
	Summary       string    `json:"summary"`
	Proxied       bool      `json:"proxied"`
	ProxyDocs     string    `json:"proxy_docs,omitempty"`
	Steps         []Step    `json:"steps"`
	DNS           DNSView   `json:"dns"`
	Certificate   CertView  `json:"certificate"`
	CLI           []string  `json:"cli"`
	GuideHref     string    `json:"guide_href"`
	CheckedAt     time.Time `json:"checked_at"`
}

// Evaluate builds the checklist and the downtime verdict.
func Evaluate(f Facts, o Options) Result {
	o = o.withDefaults()
	res := Result{Domain: f.Domain, GuideHref: GuideHref, CheckedAt: time.Now().UTC(), Proxied: f.Proxied}
	if f.Proxied {
		res.ProxyDocs = CloudflareDocs
	}
	app := appStep(f)
	cert := certStep(f, o)
	dns, view := currentDNSStep(f, o)
	sw := switchStep(f, o, view)
	auth := authStep(f, o, sw.State)
	live := liveStep(f, auth.State)
	res.Steps = []Step{app, cert, dns, sw, auth, live}
	res.DNS = view
	res.Certificate = CertView{State: certStateOf(f), Issuer: f.CertIssuer, Error: f.CertError, Explanation: f.CertExplanation,
		ProviderConnected: f.ProviderConnected, RunbookHref: RunbookHref}
	res.CLI = []string{"levelrail-cli domains cutover " + f.Domain}
	if f.ProviderConnected && cert.State != StatePass && cert.State != StateSkip {
		res.CLI = append(res.CLI, "levelrail-cli domains cutover "+f.Domain+" --get-certificate")
	}
	res.ReadyToSwitch = app.State == StatePass
	res.Risk = riskOf(app, cert)
	res.Summary = summarize(f, res, app, cert)
	return res
}

func (o Options) withDefaults() Options {
	if o.LowTTL <= 0 {
		o.LowTTL = 300
	}
	if o.DefaultETA <= 0 {
		o.DefaultETA = 1800
	}
	if o.CertETA <= 0 {
		o.CertETA = 120
	}
	return o
}

func riskOf(app, cert Step) Risk {
	certOK := cert.State == StatePass || cert.State == StateSkip
	switch {
	case app.State == StatePass && certOK:
		return RiskNone
	case app.State == StateWait:
		return RiskUnknown
	default:
		return RiskLikely
	}
}

func summarize(f Facts, r Result, app, cert Step) string {
	switch {
	case app.State != StatePass:
		return "Not ready to switch: the app does not answer on this server for " + f.Domain + " yet"
	case r.Risk == RiskNone && r.DNS.Points == PointsThis:
		return "Live: " + f.Domain + " already points at this server"
	case r.Risk == RiskNone:
		return "Ready to switch: the app answers on this server for " + f.Domain + " and the certificate is ready, no downtime expected"
	case cert.State == StateFail:
		return "Ready to switch: the app answers on this server for " + f.Domain + " (certificate failed, visitors would see a warning)"
	default:
		return "Ready to switch: the app answers on this server for " + f.Domain + " (certificate pending)"
	}
}

func appStep(f Facts) Step {
	s := Step{ID: StepApp, Title: "The app answers on this server"}
	switch {
	case !f.App.Done:
		s.State, s.Detail = StateWait, notEmpty(f.App.Err, "not checked yet")
	case f.App.Answered():
		s.State, s.Detail = StatePass, fmt.Sprintf("GET / returned %d when this server was asked for %s directly, DNS not involved", f.App.Status, f.Domain)
	default:
		s.State = StateFail
		s.Detail = notEmpty(f.App.Err, fmt.Sprintf("this server answered with status %d", f.App.Status))
	}
	return s
}

func certStateOf(f Facts) string {
	if f.CertState == "" {
		return CertOff
	}
	return f.CertState
}

func certStep(f Facts, o Options) Step {
	s := Step{ID: StepCertificate, Title: "The certificate is ready"}
	if f.UpstreamTLS {
		s.State, s.Detail = StateSkip, "a proxy in front of this server owns the certificate, nothing to issue here"
		return s
	}
	switch {
	case f.App.CertTrusted:
		s.State, s.Detail = StatePass, "a trusted certificate issued by "+notEmpty(f.App.CertIssuer, "a public CA")+" is served for "+f.Domain
	case certStateOf(f) == CertIssued:
		s.State, s.Detail = StatePass, "certificate issued by "+notEmpty(f.CertIssuer, "a public CA")
	case certStateOf(f) == CertFailed:
		s.State = StateFail
		s.Detail = notEmpty(f.CertExplanation, notEmpty(f.CertError, "the CA refused the request"))
	case certStateOf(f) == CertPending:
		s.State, s.ETASeconds, s.Detail = StateWait, o.CertETA, "the certificate is being requested"
	default:
		s.State = StateWait
		s.Detail = "no public certificate yet. Without one, visitors see a warning after the switch until HTTP-01 succeeds"
		if f.ProviderConnected {
			s.Detail = "no public certificate yet. Get it now through your DNS provider so it is ready before the switch"
			s.Action = &Action{Kind: "request", Label: "Get certificate before switching DNS",
				API: "POST /api/v1/domains/" + f.Domain + "/cutover/certificate", CLI: "levelrail-cli domains cutover " + f.Domain + " --get-certificate"}
		} else {
			s.Detail += ". Connect Cloudflare, Route53 or Google Cloud DNS to issue it ahead of time"
			s.Action = &Action{Kind: "link", Label: "Read the runbook", Href: RunbookHref}
		}
	}
	return s
}

func pointsOf(f Facts) string {
	switch {
	case f.DNSStatus == DNSConnected:
		return PointsThis
	case f.Proxied:
		return PointsCloudflare
	case len(f.Resolved) > 0:
		return PointsElsewhere
	default:
		return PointsNone
	}
}

func currentDNSStep(f Facts, o Options) (Step, DNSView) {
	view := DNSView{Points: pointsOf(f), Status: f.DNSStatus, Resolved: f.Resolved, TTLSeconds: f.TTLSeconds, LowerTTLTo: o.LowTTL, Records: records(f, o)}
	s := Step{ID: StepCurrentDNS, Title: "Current DNS and TTL"}
	where := map[string]string{
		PointsThis:       "already points at this server",
		PointsCloudflare: "is proxied by Cloudflare (" + strings.Join(f.Resolved, ", ") + ")",
		PointsElsewhere:  "points at the previous server (" + strings.Join(f.Resolved, ", ") + ")",
		PointsNone:       "does not resolve yet",
	}[view.Points]
	switch {
	case view.Points == PointsThis:
		s.State, s.Detail = StatePass, f.Domain+" "+where
	case f.TTLSeconds > 0 && f.TTLSeconds <= o.LowTTL:
		s.State, s.Detail = StatePass, fmt.Sprintf("%s %s. The TTL is %ds, so the switch takes effect quickly", f.Domain, where, f.TTLSeconds)
	case f.TTLSeconds > 0:
		s.State = StateWait
		s.Detail = fmt.Sprintf("%s %s. The TTL is %ds: lower it to %ds at least %s before switching so caches expire fast", f.Domain, where, f.TTLSeconds, o.LowTTL, human(f.TTLSeconds))
	case view.Points == PointsNone:
		s.State, s.Detail = StatePass, f.Domain+" "+where+", so no visitor is affected by the switch"
	default:
		s.State = StateWait
		s.Detail = fmt.Sprintf("%s %s. The TTL is unknown: lower it to %ds at your DNS host at least one old TTL before switching", f.Domain, where, o.LowTTL)
	}
	return s, view
}

func records(f Facts, o Options) []Record {
	var out []Record
	for _, ip := range f.ExpectedIPv4 {
		out = append(out, Record{Type: "A", Name: f.Domain, Value: ip, TTLSeconds: o.LowTTL})
	}
	for _, ip := range f.ExpectedIPv6 {
		out = append(out, Record{Type: "AAAA", Name: f.Domain, Value: ip, TTLSeconds: o.LowTTL})
	}
	if len(out) == 0 && f.ExpectedHost != "" {
		out = append(out, Record{Type: "CNAME", Name: f.Domain, Value: f.ExpectedHost, TTLSeconds: o.LowTTL})
	}
	return out
}

func switchStep(f Facts, o Options, v DNSView) Step {
	s := Step{ID: StepSwitch, Title: "Switch DNS to this server"}
	switch {
	case f.DNSStatus == DNSConnected || f.DNSStatus == DNSPropagating:
		s.State, s.Detail = StatePass, f.Domain+" resolves to this server"
		if f.DNSStatus == DNSPropagating {
			s.Detail = "public resolvers already answer with this server, this server's own resolver is catching up"
		}
	case len(v.Records) == 0:
		s.State, s.Detail = StateWait, "this server's public address is unknown. Set APP_PUBLIC_HOST, then change the record"
	default:
		s.State = StateWait
		s.ETASeconds = f.eta(o)
		s.Detail = "change the record to " + describe(v.Records) + ". Keep proxy on or off, both work"
	}
	return s
}

func (f Facts) eta(o Options) int {
	if f.TTLSeconds > 0 {
		return f.TTLSeconds
	}
	return o.DefaultETA
}

func describe(rs []Record) string {
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		parts = append(parts, fmt.Sprintf("%s %s", r.Type, r.Value))
	}
	return strings.Join(parts, " and ")
}

func authStep(f Facts, o Options, sw State) Step {
	s := Step{ID: StepAuthoritative, Title: "Verified from authoritative servers"}
	switch {
	case sw != StatePass:
		s.State, s.Detail = StateWait, "waiting for the DNS change"
	case !f.AuthKnown:
		s.State, s.Detail = StateWait, "the authoritative servers could not be asked yet"
	case f.AuthTotal > 0 && f.AuthHit >= f.AuthTotal:
		s.State, s.Detail = StatePass, fmt.Sprintf("%d of %d authoritative servers answer with this server", f.AuthHit, f.AuthTotal)
	default:
		s.State, s.ETASeconds = StateWait, f.eta(o)
		s.Detail = fmt.Sprintf("%d of %d authoritative servers answer with this server", f.AuthHit, f.AuthTotal)
	}
	return s
}

func liveStep(f Facts, auth State) Step {
	s := Step{ID: StepLive, Title: "Verified end to end over the real name"}
	switch {
	case auth != StatePass && !f.Live.Done:
		s.State, s.Detail = StateWait, "waiting for DNS to be verified"
	case !f.Live.Done:
		s.State, s.Detail = StateWait, notEmpty(f.Live.Err, "not checked yet")
	case f.Live.Answered() && (f.Live.CertTrusted || f.UpstreamTLS):
		s.State, s.Detail = StatePass, fmt.Sprintf("https://%s answers %d with a trusted certificate", f.Domain, f.Live.Status)
	case f.Live.Answered():
		s.State, s.Detail = StateFail, "https://"+f.Domain+" answers but the certificate is not trusted"
	default:
		s.State = StateFail
		s.Detail = notEmpty(f.Live.Err, fmt.Sprintf("https://%s answered with status %d", f.Domain, f.Live.Status))
	}
	return s
}

func human(sec int) string {
	switch {
	case sec >= 3600:
		return fmt.Sprintf("%dh", (sec+3599)/3600)
	case sec >= 60:
		return fmt.Sprintf("%d min", (sec+59)/60)
	}
	return fmt.Sprintf("%ds", sec)
}

func notEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
