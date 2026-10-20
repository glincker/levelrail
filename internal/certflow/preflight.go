package certflow

import (
	"net/mail"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/domaindoctor"
	"github.com/GLINCKER/levelrail/internal/ingress"
)

// IsRateLimitError reports whether a CA error text is a rate limit answer.
func IsRateLimitError(msg string) bool {
	l := strings.ToLower(msg)
	return strings.Contains(l, "ratelimited") || strings.Contains(l, "rate limit") || strings.Contains(l, "too many")
}

// Preflight item codes.
const (
	CodeIssuable  = "issuable"
	CodeUpstream  = "upstream_proxy"
	CodeDNS       = "dns"
	CodePorts     = "ports"
	CodeCAA       = "caa"
	CodeEmail     = "email"
	CodeRateLimit = "rate_limit"
	CodeStaging   = "staging"
)

// ItemState is the outcome of one preflight item.
type ItemState string

// ItemState values. Only fail blocks the primary action.
const (
	StatePass ItemState = "pass"
	StateWarn ItemState = "warn"
	StateFail ItemState = "fail"
	StateSkip ItemState = "skipped"
)

// Item is one preflight finding with the next step in plain words.
type Item struct {
	Code   string    `json:"code"`
	State  ItemState `json:"state"`
	Detail string    `json:"detail"`
	Fix    string    `json:"fix,omitempty"`
}

// Result is the whole preflight for one domain.
type Result struct {
	Domain  string `json:"domain"`
	Staging bool   `json:"staging"`
	// Email is the contact that will be used: the typed one or the inferred default.
	Email string `json:"email"`
	// Ready is true when no item failed.
	Ready bool   `json:"ready"`
	Items []Item `json:"items"`
	// NextSafeAt is set when a rate limit is in the way.
	NextSafeAt *time.Time `json:"next_safe_at,omitempty"`
}

// Input is everything Evaluate decides from. The caller fills Report from the
// domain doctor with ACME forced on, so CAA and port checks really run.
type Input struct {
	Domain          string
	Email           string
	DefaultEmail    string
	Staging         bool
	TLSUpstream     bool
	Report          domaindoctor.Report
	Existing        []Existing
	History         Snapshot
	Limits          Limits
	LastCAError     string
	LastCAErrorTime time.Time
	Now             time.Time
}

// Evaluate turns the inputs into a verdict. It performs no network calls.
func Evaluate(in Input) Result {
	res := Result{Domain: in.Domain, Staging: in.Staging, Email: strings.TrimSpace(in.Email)}
	if res.Email == "" {
		res.Email = strings.TrimSpace(in.DefaultEmail)
	}
	res.Items = append(res.Items, issuableItem(in), upstreamItem(in))
	res.Items = append(res.Items, reportItems(in.Report)...)
	res.Items = append(res.Items, emailItem(res.Email))
	rate, next := rateItem(in)
	res.Items = append(res.Items, rate)
	if next != nil {
		res.NextSafeAt = next
	}
	if in.Staging {
		if c := StagingConflict(in.Domain, in.Existing); c != nil {
			res.Items = append(res.Items, Item{Code: CodeReplaces, State: StateFail, Detail: c.Error(), Fix: "Request the real certificate without staging."})
		}
		res.Items = append(res.Items, Item{Code: CodeStaging, State: StateWarn,
			Detail: "Staging certificates are signed by a test CA that browsers do not trust. Use staging only to rehearse; switch to a real certificate afterwards."})
	}
	res.Ready = true
	for _, it := range res.Items {
		if it.State == StateFail {
			res.Ready = false
		}
	}
	return res
}

func issuableItem(in Input) Item {
	if ingress.ACMEIssuable(in.Domain) {
		return Item{Code: CodeIssuable, State: StatePass, Detail: in.Domain + " is a public name a CA can sign."}
	}
	return Item{Code: CodeIssuable, State: StateFail,
		Detail: in.Domain + " cannot get a public certificate (IP addresses, single labels and reserved names such as .local or example.com are never signed).",
		Fix:    "Use a real domain you own."}
}

func upstreamItem(in Input) Item {
	if !in.TLSUpstream {
		return Item{Code: CodeUpstream, State: StatePass, Detail: "This server terminates TLS itself."}
	}
	return Item{Code: CodeUpstream, State: StateFail,
		Detail: "A proxy in front of this server terminates TLS, so this server never asks a CA for a certificate.",
		Fix:    "Turn off TLS terminated upstream in Settings, Domains, or keep the certificate on the proxy."}
}

// reportItems maps the doctor's DNS, CAA and port checks onto preflight items.
func reportItems(rep domaindoctor.Report) []Item {
	byID := map[string]domaindoctor.Check{}
	for _, c := range rep.Checks {
		byID[c.ID] = c
	}
	return []Item{dnsItem(byID, rep), portsItem(byID, rep), caaItem(byID)}
}

func dnsItem(byID map[string]domaindoctor.Check, rep domaindoctor.Report) Item {
	it := Item{Code: CodeDNS, State: StatePass, Detail: "The domain points at this server."}
	for _, id := range []string{domaindoctor.IDDNSResolves, domaindoctor.IDDNSPointsHere} {
		c, ok := byID[id]
		if !ok || c.State == domaindoctor.StatePass {
			continue
		}
		if c.State == domaindoctor.StateFail || c.State == domaindoctor.StateWarn {
			it.State, it.Detail = StateFail, c.Title
			if c.Detail != "" {
				it.Detail += ": " + c.Detail
			}
			if c.Fix != nil {
				it.Fix = c.Fix.Summary
			}
			return it
		}
		if c.State == domaindoctor.StateUnavailable {
			it.State, it.Detail = StateWarn, "DNS could not be checked just now: "+c.Title
		}
	}
	if len(rep.Checks) == 0 {
		return Item{Code: CodeDNS, State: StateWarn, Detail: "DNS was not checked."}
	}
	return it
}

func portsItem(byID map[string]domaindoctor.Check, rep domaindoctor.Report) Item {
	it := Item{Code: CodePorts, State: StatePass, Detail: "Port 80 answers on this server's address."}
	for _, id := range []string{domaindoctor.IDProxyPorts, domaindoctor.IDPort80} {
		c, ok := byID[id]
		if !ok {
			continue
		}
		switch c.State {
		case domaindoctor.StateFail, domaindoctor.StateWarn:
			it.State, it.Detail = StateFail, c.Title
			if c.Detail != "" {
				it.Detail += ": " + c.Detail
			}
			if c.Fix != nil {
				it.Fix = c.Fix.Summary
			}
			return it
		case domaindoctor.StateSkipped:
			it.State = StateWarn
			it.Detail = "Port 80 was not tested from here. Let's Encrypt connects to it from the internet, so make sure it is open in the firewall and cloud security group."
			if rep.ProbeNote != "" {
				it.Detail += " (" + rep.ProbeNote + ")"
			}
		}
	}
	if it.State == StatePass {
		it.Detail += " The check runs from this server, so it cannot prove the internet can reach it."
	}
	return it
}

func caaItem(byID map[string]domaindoctor.Check) Item {
	c, ok := byID[domaindoctor.IDCAA]
	switch {
	case !ok || c.State == domaindoctor.StateSkipped || c.State == domaindoctor.StateUnavailable:
		return Item{Code: CodeCAA, State: StateWarn, Detail: "CAA records could not be read. A CAA record that omits Let's Encrypt would make issuance fail."}
	case c.State == domaindoctor.StateFail:
		it := Item{Code: CodeCAA, State: StateFail, Detail: c.Title + ": " + c.Detail}
		if c.Fix != nil {
			it.Fix = c.Fix.Summary
		}
		return it
	}
	return Item{Code: CodeCAA, State: StatePass, Detail: "No CAA record blocks Let's Encrypt."}
}

func emailItem(email string) Item {
	if email == "" {
		return Item{Code: CodeEmail, State: StateFail, Detail: "No contact email is set. Let's Encrypt sends expiry notices to it.", Fix: "Enter a reachable address."}
	}
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email {
		return Item{Code: CodeEmail, State: StateFail, Detail: "The contact email is not a valid address.", Fix: "Enter an address like you@yourdomain.com."}
	}
	if err := ingress.ValidateACMEContact(email); err != nil {
		return Item{Code: CodeEmail, State: StateFail, Detail: err.Error(), Fix: "Use a real, reachable address."}
	}
	return Item{Code: CodeEmail, State: StatePass, Detail: "Expiry notices go to " + email + "."}
}

func rateItem(in Input) (Item, *time.Time) {
	if in.Staging {
		return Item{Code: CodeRateLimit, State: StateSkip, Detail: "Staging has its own, much higher limits."}, nil
	}
	v := in.History.Check(in.Limits, in.Now, in.LastCAError, in.LastCAErrorTime)
	if !v.Blocked {
		return Item{Code: CodeRateLimit, State: StatePass, Detail: "No Let's Encrypt limit is close for this name."}, nil
	}
	next := v.NextSafeAt
	return Item{Code: CodeRateLimit, State: StateFail,
		Detail: rateDetail(v.Reason),
		Fix:    "The next safe time is " + next.UTC().Format(time.RFC3339) + ". Try staging meanwhile to rehearse."}, &next
}

func rateDetail(reason string) string {
	switch reason {
	case ReasonFailedValidations:
		return "Too many failed validations for this name in the last hour; Let's Encrypt would refuse another attempt."
	case ReasonDuplicates:
		return "This exact name already got the maximum number of certificates this week."
	}
	return "Let's Encrypt recently answered with a rate limit for this name."
}
