package email

import (
	"context"
	"errors"
	"net"
	"net/textproto"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

// Defaults for the circuit breaker; the boot code overrides them from env.
const (
	DefaultFailoverThreshold = 3
	DefaultFailoverCooldown  = 60 * time.Second
)

// ProviderConfig is one configured sending provider: a stable id plus the Config built from its row and credentials.
// The id is empty for the env fallback and the legacy single-row path, which never fail over.
type ProviderConfig struct {
	ID     string
	Config Config
}

// ProvidersLoader resolves the ordered provider list on every send: primary first, then the enabled backup.
type ProvidersLoader func(ctx context.Context) ([]ProviderConfig, error)

// preSendError marks a failure that happened before the message body was handed to the provider,
// so the provider cannot have accepted the mail.
type preSendError struct{ err error }

func (e *preSendError) Error() string { return e.err.Error() }
func (e *preSendError) Unwrap() error { return e.err }

func preSend(err error) error {
	if err == nil {
		return nil
	}
	return &preSendError{err: err}
}

// FailoverEligible reports whether err is a pre-send provider failure that justifies trying the backup.
// When it is unclear whether the body reached the provider it returns false, so mail is never duplicated.
func FailoverEligible(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, ErrRateLimited) || errors.Is(err, ErrNotConfigured) {
		return false
	}
	var he *HTTPError
	if errors.As(err, &he) {
		if he.Status == 429 || he.Status >= 500 {
			return true
		}
		return providerSideKind(Classify(err))
	}
	var sc interface{ HTTPStatusCode() int }
	if errors.As(err, &sc) {
		if st := sc.HTTPStatusCode(); st == 429 || st >= 500 {
			return true
		}
	}
	if providerSideKind(Classify(err)) {
		return true
	}
	var pe *textproto.Error
	if errors.As(err, &pe) {
		return false // a protocol reply is a verdict on the recipient or content
	}
	var pre *preSendError
	if errors.As(err, &pre) {
		return true
	}
	var op *net.OpError
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EHOSTUNREACH) || errors.Is(err, syscall.ENETUNREACH) ||
		(errors.As(err, &op) && op.Op == "dial")
}

func providerSideKind(k FailureKind) bool {
	switch k {
	case FailAuth, FailTLS, FailSenderNotVerified, FailRegionMismatch:
		return true
	}
	return false
}

// Circuit states.
const (
	circuitClosed = iota
	circuitOpen
	circuitHalfOpen
)

type circuitState struct {
	state    int
	failures int
	openedAt time.Time
}

// Outcomes reported to the breaker after a provider attempt.
type outcome int

const (
	outcomeOK      outcome = iota // provider answered; the send may still have been refused for a recipient reason
	outcomeFail                   // failover-class failure
	outcomeNeutral                // nothing learned about the provider (own cap, cancel)
)

// breaker is the in-memory per-provider circuit. A restart resets every circuit.
type breaker struct {
	mu        sync.Mutex
	threshold int
	cooldown  time.Duration
	now       func() time.Time
	circuits  map[string]*circuitState
}

func newBreaker(threshold int, cooldown time.Duration) *breaker {
	return &breaker{threshold: threshold, cooldown: cooldown, now: time.Now, circuits: map[string]*circuitState{}}
}

func (b *breaker) get(id string) *circuitState {
	c, ok := b.circuits[id]
	if !ok {
		c = &circuitState{}
		b.circuits[id] = c
	}
	return c
}

// allow reports whether a send may go to id. After the cooldown exactly one caller gets the half-open probe.
func (b *breaker) allow(id string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.get(id)
	switch c.state {
	case circuitOpen:
		if b.now().Sub(c.openedAt) >= b.cooldown {
			c.state = circuitHalfOpen
			return true
		}
		return false
	case circuitHalfOpen:
		return false
	}
	return true
}

func (b *breaker) report(id string, o outcome) {
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.get(id)
	switch o {
	case outcomeOK:
		*c = circuitState{}
	case outcomeFail:
		c.failures++
		if c.state == circuitHalfOpen || c.failures >= b.threshold {
			c.state = circuitOpen
			c.openedAt = b.now()
		}
	case outcomeNeutral:
		if c.state == circuitHalfOpen {
			c.state = circuitOpen // release the probe slot; the next one waits for a fresh cooldown check
		}
	}
}

const maxProviderText = 512

var (
	emailInText = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	bearerToken = regexp.MustCompile(`(?i)(bearer|basic|api[-_ ]?key|token)[\s:=]+[A-Za-z0-9._~+/=\-]{8,}`)
)

const redacted = "[redacted]"

// RedactProviderText truncates provider output and strips credentials and recipient addresses from it.
// secrets are exact values (passwords, API keys) to remove wherever they appear.
func RedactProviderText(s string, secrets ...string) string {
	for _, sec := range secrets {
		if len(sec) >= 4 {
			s = strings.ReplaceAll(s, sec, redacted)
		}
	}
	s = bearerToken.ReplaceAllString(s, "$1 "+redacted)
	s = emailInText.ReplaceAllString(s, redacted)
	if len(s) > maxProviderText {
		cut := maxProviderText
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut] + "..."
	}
	return s
}

// errorText is err's message with an HTTP provider body run through RedactProviderText and secrets removed.
func errorText(err error, secrets []string) string {
	text := err.Error()
	var he *HTTPError
	if errors.As(err, &he) && he.Body != "" {
		text = strings.Replace(text, he.Body, RedactProviderText(he.Body, secrets...), 1)
	}
	for _, sec := range secrets {
		if len(sec) >= 4 {
			text = strings.ReplaceAll(text, sec, redacted)
		}
	}
	return text
}

// configSecrets lists the credential values in cfg, for redaction.
func configSecrets(cfg Config) []string {
	var out []string
	if cfg.SMTP != nil {
		out = append(out, cfg.SMTP.Password)
	}
	if cfg.SES != nil {
		out = append(out, cfg.SES.SecretAccessKey, cfg.SES.AccessKeyID)
	}
	if cfg.Resend != nil {
		out = append(out, cfg.Resend.APIKey)
	}
	if cfg.API != nil {
		out = append(out, cfg.API.APIKey)
	}
	return out
}
