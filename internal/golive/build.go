package golive

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/domaindoctor"
	"github.com/GLINCKER/levelrail/internal/fixit"
	"github.com/GLINCKER/levelrail/internal/hostfw"
)

// Build assembles the checklist. It is pure: every fact comes in through in.
func Build(in Input) Checklist {
	if in.Now.IsZero() {
		in.Now = time.Now()
	}
	if in.PollEvery <= 0 {
		in.PollEvery = 10 * time.Second
	}
	steps := []Step{appStep(in), portStep(in), firewallStep(in), dnsStep(in)}
	steps = append(steps, certStep(in, steps[3]), liveStep(in, steps))
	cl := Checklist{App: in.App, Domain: in.Domain, Steps: steps, CheckedAt: in.Now.UTC()}
	cl.State = StateOf(steps)
	cl.Live = cl.State == StateLive
	for _, s := range steps {
		if s.Status != StatusOK {
			cl.Next = s.ID
			break
		}
	}
	if in.Domain != "" {
		cl.URL = "https://" + in.Domain
	}
	cl.Timeline = buildTimeline(in, steps)
	return cl
}

// StateOf collapses the steps into the badge state.
func StateOf(steps []Step) State {
	by := map[string]Status{}
	for _, s := range steps {
		by[s.ID] = s.Status
	}
	switch {
	case by[StepApp] == StatusFail || by[StepFirewall] == StatusFail:
		return StateNeedsAttention
	case by[StepApp] != StatusOK:
		return StateNotLive
	case by[StepDNS] != StatusOK:
		return StateWaitingDNS
	case by[StepCertificate] == StatusFail:
		return StateNeedsAttention
	case by[StepCertificate] != StatusOK:
		return StateWaitingCertificate
	case by[StepLive] == StatusOK:
		return StateLive
	case by[StepLive] == StatusFail || by[StepLive] == StatusWarn:
		return StateNeedsAttention
	}
	return StateWaitingCertificate
}

func recheck() *Action { return &Action{Kind: ActionRecheck, Label: "Check again"} }

func ev(label, value string) Evidence { return Evidence{Label: label, Value: value} }

func appStep(in Input) Step {
	s := Step{ID: StepApp, Title: "App is running and healthy", Evidence: []Evidence{}}
	a := in.AppState
	switch {
	case !a.Known:
		s.Status, s.Summary = StatusPending, "No deploy has finished yet, so there is nothing to put online."
		s.Primary = &Action{Kind: ActionLink, Label: "Open deploys", Href: "/apps/" + url.PathEscape(in.App) + "/deploys"}
	case !a.Running:
		s.Status, s.Summary = StatusFail, "The app is not running."
		s.Primary = &Action{Kind: ActionLink, Label: "See why the deploy failed", Href: a.DiagnosisHref}
	case !a.Ready:
		s.Status, s.Summary = StatusWarn, "The app is running but is not passing its readiness check yet."
		s.Primary = &Action{Kind: ActionLink, Label: "See why it is not ready", Href: a.DiagnosisHref}
	default:
		s.Status, s.Summary = StatusOK, "The app is running and passing its readiness check."
	}
	if a.Detail != "" {
		s.Evidence = append(s.Evidence, ev("Status", a.Detail))
	}
	if s.Primary != nil && s.Primary.Href == "" && s.Primary.Kind == ActionLink {
		s.Primary = recheck()
	}
	return s
}

func portStep(in Input) Step {
	p := in.Port
	s := Step{ID: StepPort, Title: "Reachable on its port", Evidence: []Evidence{}}
	if p.ZeroConfigURL != "" {
		s.Evidence = append(s.Evidence, Evidence{Label: "Automatic address", Value: p.ZeroConfigURL, Copy: true})
	}
	if p.DirectURL != "" {
		s.Evidence = append(s.Evidence, Evidence{Label: "Direct host and port", Value: p.DirectURL, Copy: true})
	}
	switch {
	case !in.AppState.Running:
		s.Status, s.Summary, s.Primary = StatusPending, "Waiting for the app to run before its port can be tested.", recheck()
	case p.Probe != nil && !p.Probe.OK:
		s.Status, s.Summary = StatusFail, "The app is running but nothing answered on its port: "+orDefault(p.Probe.Failure, "no response")+"."
		s.Primary = recheck()
	case p.Probe != nil:
		s.Status, s.Summary = StatusOK, fmt.Sprintf("The app answered on its port (HTTP %d).", p.Probe.Status)
	case p.ZeroConfigURL == "" && p.DirectURL == "":
		s.Status, s.Summary, s.Primary = StatusUnknown, "No address could be worked out for this app yet.", recheck()
	default:
		s.Status, s.Summary = StatusOK, "The app has an address you can open."
	}
	if p.DirectURL != "" {
		s.Summary += " The direct port is plain HTTP and only for testing: do not use it as the public address."
	}
	if s.Primary == nil {
		s.Primary = portPrimary(p)
	}
	return s
}

func portPrimary(p PortInput) *Action {
	switch {
	case p.ZeroConfigURL != "":
		return &Action{Kind: ActionLink, Label: "Open the app", Href: p.ZeroConfigURL}
	case p.DirectAllowed && p.NetworkHref != "":
		return &Action{Kind: ActionLink, Label: "Choose a test port", Href: p.NetworkHref}
	}
	return recheck()
}

func orDefault(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}

func checkByID(rep *domaindoctor.Report, id string) (domaindoctor.Check, bool) {
	if rep == nil {
		return domaindoctor.Check{}, false
	}
	for _, c := range rep.Checks {
		if c.ID == id {
			return c, true
		}
	}
	return domaindoctor.Check{}, false
}

func firewallStep(in Input) Step {
	f := in.Firewall
	s := Step{ID: StepFirewall, Title: "Server firewall allows ports 80 and 443", Evidence: []Evidence{}, Provider: f.Provider}
	s.Evidence = append(s.Evidence, ev("Host firewall", string(f.Host.Backend)), ev("Detail", f.Host.Detail))
	probeFailed := false
	for _, id := range []string{domaindoctor.IDProxyPorts, domaindoctor.IDPort80} {
		if c, ok := checkByID(in.Report, id); ok && c.State == domaindoctor.StateFail {
			probeFailed = true
			s.Evidence = append(s.Evidence, ev("Port test", c.Title))
		}
	}
	switch {
	case f.Host.Unknown:
		s.Status, s.Summary = StatusUnknown, "The host firewall could not be read from here: "+f.Host.Detail
		s.Primary = recheck()
	case f.Host.Blocking():
		s.Status, s.Summary = StatusFail, "The host firewall is blocking "+portsText(f.Host.MissingPorts())+", so visitors and Let's Encrypt cannot reach this server."
		s.FirewallFix = fixPreview(f)
		s.Primary = firewallPrimary(f)
	case probeFailed:
		s.Status, s.Summary = StatusFail, "The host firewall is not blocking, but port 80 does not answer on this server's address. A cloud or provider firewall is the likely cause."
		s.Primary = providerPrimary(f)
	default:
		s.Status, s.Summary = StatusOK, okFirewallText(f.Host)
	}
	if s.Provider != nil && s.Status != StatusOK {
		s.DocsURL = s.Provider.DocsURL
	}
	return s
}

func okFirewallText(h hostfw.Status) string {
	switch {
	case h.Backend == hostfw.BackendNone:
		return "No host firewall tool is visible, so nothing on this machine blocks ports 80 and 443. Provider firewalls are checked by the next steps."
	case !h.Active:
		return "The host firewall is off or allows everything, so it is not blocking ports 80 and 443. It was left as it is."
	}
	return "The host firewall allows ports 80 and 443."
}

func firewallPrimary(f FirewallInput) *Action {
	if f.FixKind == fixit.KindAuto && f.Plan.Refusal == "" && len(f.Plan.Add) > 0 {
		return &Action{Kind: ActionFix, Label: "Open ports 80 and 443", FixID: f.FixID}
	}
	return recheck()
}

func providerPrimary(f FirewallInput) *Action {
	if f.Provider != nil {
		return &Action{Kind: ActionLink, Label: "Open " + f.Provider.Name + " firewall steps", Href: f.Provider.DocsURL}
	}
	return recheck()
}

func fixPreview(f FirewallInput) *FixPreview {
	p := &FixPreview{FixID: f.FixID, Kind: string(f.FixKind), Persistent: f.Plan.Persistent, Note: f.Plan.Note, Refusal: f.Plan.Refusal,
		Commands: []string{}, Undo: []string{}}
	for _, c := range f.Plan.Add {
		p.Commands = append(p.Commands, c.String())
	}
	for _, c := range f.Plan.Undo {
		p.Undo = append(p.Undo, c.String())
	}
	return p
}

func portsText(ports []int) string {
	parts := make([]string, len(ports))
	for i, p := range ports {
		parts[i] = fmt.Sprint(p)
	}
	return "port " + strings.Join(parts, " and ")
}
