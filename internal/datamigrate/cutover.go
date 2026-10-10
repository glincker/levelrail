package datamigrate

import (
	"fmt"
	"net/netip"
	"strings"
)

// Check outcomes.
const (
	CheckPass = "pass"
	CheckWarn = "warn"
	CheckFail = "fail"
)

// Cutover verdicts.
const (
	VerdictGo     = "go"
	VerdictWait   = "wait"
	VerdictNoGo   = "no-go"
	VerdictSwitch = "switched"
)

// DefaultMaxTTL is the TTL in seconds above which a cutover should wait.
const DefaultMaxTTL = 300

// Check is one named result with what to do about it.
type Check struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// DomainState is what public DNS says about a domain right now.
type DomainState struct {
	Addrs []string `json:"addrs,omitempty"`
	CNAME string   `json:"cname,omitempty"`
	// TTL is the TTL in seconds of the record the operator edits, 0 when unknown.
	TTL uint32 `json:"ttl"`
	// Authoritative is false when TTL came from a caching resolver, so the
	// real value may be higher than what it shows.
	Authoritative bool   `json:"authoritative"`
	Error         string `json:"error,omitempty"`
}

// RecordChange is the exact DNS edit to make.
type RecordChange struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Value    string `json:"value"`
	TTL      uint32 `json:"ttl"`
	Replaces string `json:"replaces,omitempty"`
}

// EvalInput is everything a cutover decision for one domain depends on.
type EvalInput struct {
	App         string
	Domain      string
	Ready       bool
	ReadyDetail string
	DNS         DomainState
	TargetIPs   []string
	MaxTTL      uint32
}

// DomainPlan is the go or no-go for one domain.
type DomainPlan struct {
	App     string        `json:"app"`
	Domain  string        `json:"domain"`
	Verdict string        `json:"verdict"`
	Checks  []Check       `json:"checks"`
	DNS     DomainState   `json:"dns"`
	Change  *RecordChange `json:"change,omitempty"`
}

// Evaluate decides whether switching domain's DNS to this platform is safe.
// The source keeps serving until the operator makes the change.
func Evaluate(in EvalInput) DomainPlan {
	maxTTL := in.MaxTTL
	if maxTTL == 0 {
		maxTTL = DefaultMaxTTL
	}
	plan := DomainPlan{App: in.App, Domain: in.Domain, DNS: in.DNS}

	ready := Check{ID: "readiness", Status: CheckPass, Detail: "the app is healthy here: " + in.ReadyDetail}
	if !in.Ready {
		ready = Check{ID: "readiness", Status: CheckFail, Detail: "the app is not healthy here yet: " + in.ReadyDetail,
			Fix: "wait for the readiness probe to pass, or check the app's logs and deploy status"}
	}
	plan.Checks = append(plan.Checks, ready)

	here := pointsAtTarget(in.DNS.Addrs, in.TargetIPs)
	plan.Checks = append(plan.Checks, pointingCheck(in, here))
	ttl := ttlCheck(in, here, maxTTL)
	plan.Checks = append(plan.Checks, ttl)

	switch {
	case !in.Ready:
		plan.Verdict = VerdictNoGo
	case here:
		plan.Verdict = VerdictSwitch
	case ttl.Status != CheckPass:
		plan.Verdict = VerdictWait
	default:
		plan.Verdict = VerdictGo
	}
	if !here && len(in.TargetIPs) > 0 {
		plan.Change = recordChange(in, maxTTL)
	}
	return plan
}

func pointsAtTarget(addrs, targets []string) bool {
	for _, a := range addrs {
		for _, t := range targets {
			if a == t {
				return true
			}
		}
	}
	return false
}

func pointingCheck(in EvalInput, here bool) Check {
	switch {
	case in.DNS.Error != "":
		return Check{ID: "dns-target", Status: CheckWarn, Detail: "could not resolve " + in.Domain + ": " + in.DNS.Error,
			Fix: "check the record exists at your DNS provider"}
	case here:
		return Check{ID: "dns-target", Status: CheckPass, Detail: in.Domain + " already resolves to this node"}
	case len(in.DNS.Addrs) == 0:
		return Check{ID: "dns-target", Status: CheckWarn, Detail: in.Domain + " has no address record yet"}
	}
	d := in.Domain + " resolves to " + strings.Join(in.DNS.Addrs, ", ")
	if in.DNS.CNAME != "" {
		d += " via CNAME " + in.DNS.CNAME
	}
	return Check{ID: "dns-target", Status: CheckPass, Detail: d + ", the source keeps serving until you switch"}
}

func ttlCheck(in EvalInput, here bool, maxTTL uint32) Check {
	if here {
		return Check{ID: "dns-ttl", Status: CheckPass, Detail: "already switched, TTL no longer matters"}
	}
	switch {
	case in.DNS.Error != "" || (in.DNS.TTL == 0 && len(in.DNS.Addrs) == 0):
		return Check{ID: "dns-ttl", Status: CheckWarn, Detail: "TTL is unknown"}
	case in.DNS.TTL > maxTTL:
		return Check{ID: "dns-ttl", Status: CheckWarn,
			Detail: fmt.Sprintf("TTL is %ds, above %ds: clients keep the old address for up to that long after the switch", in.DNS.TTL, maxTTL),
			Fix:    fmt.Sprintf("set the TTL of %s to %d at your DNS provider, then wait %d seconds before switching", in.Domain, maxTTL, in.DNS.TTL)}
	}
	d := fmt.Sprintf("TTL is %ds", in.DNS.TTL)
	if !in.DNS.Authoritative {
		d += " as seen by a caching resolver, the configured value may be higher"
	}
	return Check{ID: "dns-ttl", Status: CheckPass, Detail: d}
}

func recordChange(in EvalInput, ttl uint32) *RecordChange {
	ip := in.TargetIPs[0]
	typ := "A"
	if a, err := netip.ParseAddr(ip); err == nil && a.Is6() {
		typ = "AAAA"
	}
	c := &RecordChange{Type: typ, Name: in.Domain, Value: ip, TTL: ttl}
	switch {
	case in.DNS.CNAME != "":
		c.Replaces = "CNAME " + in.DNS.CNAME + " (remove it, a name cannot hold a CNAME and an address)"
	case len(in.DNS.Addrs) > 0:
		c.Replaces = strings.Join(in.DNS.Addrs, ", ")
	}
	return c
}

// Overall is the verdict for a whole migration: the worst of its domains.
func Overall(plans []DomainPlan) string {
	if len(plans) == 0 {
		return VerdictNoGo
	}
	rank := map[string]int{VerdictSwitch: 0, VerdictGo: 1, VerdictWait: 2, VerdictNoGo: 3}
	worst := VerdictSwitch
	for _, p := range plans {
		if rank[p.Verdict] > rank[worst] {
			worst = p.Verdict
		}
	}
	return worst
}
