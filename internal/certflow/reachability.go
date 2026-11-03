package certflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// EnvProofTTL is how long a reachability answer is reused before the CA is asked again.
const EnvProofTTL = "APP_REACH_PROOF_TTL"

const defaultProofTTL = 10 * time.Minute

// Outcome is what the staging order proved about port 80 from the outside.
type Outcome string

// Outcome values. Limited means no order was placed, so nothing was proven.
const (
	OutcomeOpen    Outcome = "open"
	OutcomeClosed  Outcome = "closed"
	OutcomeUnknown Outcome = "unknown"
	OutcomeLimited Outcome = "limited"
)

// ValidationError means the CA was reached and its HTTP-01 check failed.
type ValidationError struct{ Detail string }

func (e *ValidationError) Error() string { return e.Detail }

// Orderer places one staging order for domain and answers the HTTP-01
// challenge on port 80. A nil error means the CA validated it. A
// *ValidationError means the CA ran the check and it failed; any other error
// means the CA could not be asked.
type Orderer interface {
	ProveHTTP01(ctx context.Context, directory, domain string) error
}

// Proof is the verdict shown on the ports step.
type Proof struct {
	Domain     string     `json:"domain"`
	Outcome    Outcome    `json:"outcome"`
	Detail     string     `json:"detail"`
	CAReason   string     `json:"ca_reason,omitempty"`
	NextSafeAt *time.Time `json:"next_safe_at,omitempty"`
	At         time.Time  `json:"at"`
}

// Prover asks a staging CA to reach this server, within the ledger and attempt limits.
type Prover struct {
	orderer   Orderer
	directory string
	ledger    *Ledger
	limits    Limits
	ttl       time.Duration

	mu       sync.Mutex
	last     map[string]Proof
	attempts map[string][]time.Time
}

// ProofTTLFromEnv reads the cache lifetime of an answer.
func ProofTTLFromEnv() time.Duration {
	if d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(EnvProofTTL))); err == nil && d > 0 {
		return d
	}
	return defaultProofTTL
}

// NewProver refuses any directory that is not a staging one, so a proof can
// never spend production certificates.
func NewProver(o Orderer, directory string, led *Ledger, lim Limits, ttl time.Duration) (*Prover, error) {
	if o == nil || led == nil {
		return nil, errors.New("certflow: prover needs an orderer and a ledger")
	}
	if !strings.Contains(strings.ToLower(directory), "staging") {
		return nil, fmt.Errorf("certflow: %q is not a staging directory", directory)
	}
	return &Prover{orderer: o, directory: directory, ledger: led, limits: lim, ttl: ttl,
		last: map[string]Proof{}, attempts: map[string][]time.Time{}}, nil
}

// Prove returns a recent answer when there is one, else places a staging order.
// perHour is the domains.https_attempts_per_hour guard.
func (p *Prover) Prove(ctx context.Context, domain string, now time.Time, perHour int, lastCAErr string, lastCAErrAt time.Time) Proof {
	domain = strings.ToLower(strings.TrimSpace(domain))
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.last[domain]; ok && now.Sub(c.At) < p.ttl {
		return c
	}
	if v := p.ledger.Snapshot(domain).Check(p.limits, now, lastCAErr, lastCAErrAt); v.Blocked {
		next := v.NextSafeAt
		return Proof{Domain: domain, Outcome: OutcomeLimited, At: now, NextSafeAt: &next,
			Detail: "Not tested from the internet yet: " + rateDetail(v.Reason) + " Trying again now could make it worse."}
	}
	recent := p.attempts[domain][:0]
	for _, t := range p.attempts[domain] {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	p.attempts[domain] = recent
	if perHour > 0 && len(recent) >= perHour {
		next := recent[0].Add(time.Hour)
		return Proof{Domain: domain, Outcome: OutcomeLimited, At: now, NextSafeAt: &next,
			Detail: "Not tested from the internet yet: too many tests for this name in the last hour."}
	}
	p.attempts[domain] = append(recent, now)
	out := classify(domain, now, p.orderer.ProveHTTP01(ctx, p.directory, domain))
	p.last[domain] = out
	return out
}

func classify(domain string, now time.Time, err error) Proof {
	pr := Proof{Domain: domain, At: now}
	var ve *ValidationError
	switch {
	case err == nil:
		pr.Outcome = OutcomeOpen
		pr.Detail = "A certificate authority reached port 80 on this server from the internet."
	case errors.As(err, &ve):
		pr.Outcome, pr.CAReason = OutcomeClosed, ve.Detail
		pr.Detail = closedWords(ve.Detail)
	default:
		pr.Outcome = OutcomeUnknown
		pr.Detail = "The certificate authority's test server could not be reached, so port 80 was not tested from the internet."
	}
	return pr
}

// closedWords turns the CA's text into what to do about it.
func closedWords(reason string) string {
	l := strings.ToLower(reason)
	switch {
	case strings.Contains(l, "connection refused"):
		return "The internet reached this address but nothing accepted the connection on port 80. Start the web proxy, or open port 80 in the firewall."
	case strings.Contains(l, "timeout") || strings.Contains(l, "timed out") || strings.Contains(l, "i/o timeout"):
		return "A certificate authority could not connect to port 80: the connection timed out. A firewall or cloud security group is likely dropping it."
	case strings.Contains(l, "no such host") || strings.Contains(l, "no valid a records") || strings.Contains(l, "nxdomain"):
		return "The domain does not point at a public address yet, so the certificate authority had nowhere to connect."
	}
	return "A certificate authority could not validate this domain over port 80."
}
