package trafficpolicy

import (
	"fmt"
	"strings"
	"time"
)

// KindShield is the per-domain bot and abuse defense section.
const KindShield = "shield"

// ShieldReason names why the shield turned a request away.
type ShieldReason int

// Shield reasons, in the order their counters are stored.
const (
	ShieldRateLimit ShieldReason = iota
	ShieldConcurrency
	ShieldBadBot
	ShieldBodyTooLarge
	ShieldReasonCount
)

var shieldReasonNames = [ShieldReasonCount]string{"rate_limit", "concurrency", "bad_bot", "body_too_large"}

// String is the reason's stable wire and metric name.
func (r ShieldReason) String() string {
	if r < 0 || r >= ShieldReasonCount {
		return "unknown"
	}
	return shieldReasonNames[r]
}

// Shield limits how much one client can cost a domain. Zero values turn an
// individual protection off.
type Shield struct {
	Enabled bool `json:"enabled"`
	// RatePerMinute is the sustained per-client-IP request rate; Burst is the
	// bucket size (default RatePerMinute/6, at least 1).
	RatePerMinute int `json:"rate_per_minute,omitempty"`
	Burst         int `json:"burst,omitempty"`
	// MaxConcurrent caps in-flight requests per client IP.
	MaxConcurrent int `json:"max_concurrent,omitempty"`
	// MaxBodyBytes rejects larger request bodies with 413.
	MaxBodyBytes int64 `json:"max_body_bytes,omitempty"`
	// BlockBots turns away user agents on the shipped denylist plus BotAgents.
	BlockBots bool     `json:"block_bots,omitempty"`
	BotAgents []string `json:"bot_agents,omitempty"`
	// Allow lists addresses and CIDRs the shield never touches.
	Allow []string `json:"allow,omitempty"`
	// RetryAfterSeconds is sent with 429 (default from the instance limits).
	RetryAfterSeconds int `json:"retry_after_seconds,omitempty"`
	// TarpitMillis delays a rejection, holding the client's connection; the
	// instance caps both the delay and the number of delayed requests.
	TarpitMillis int `json:"tarpit_ms,omitempty"`
	// UnderAttackUntil tightens every limit until that time, then lapses.
	UnderAttackUntil *time.Time `json:"under_attack_until,omitempty"`
}

// UnderAttack reports whether emergency mode is on at now.
func (s Shield) UnderAttack(now time.Time) bool {
	return s.UnderAttackUntil != nil && now.Before(*s.UnderAttackUntil)
}

// ShieldEffective is what the handler enforces at one instant.
type ShieldEffective struct {
	Active        bool
	UnderAttack   bool
	RatePerMinute int
	Burst         int
	MaxConcurrent int
	MaxBodyBytes  int64
	BlockBots     bool
	RetryAfter    int
	Tarpit        time.Duration
}

// Effective resolves the shield at now, applying the under-attack clamp.
func (s Shield) Effective(now time.Time, l Limits) ShieldEffective {
	ua := s.UnderAttack(now)
	e := ShieldEffective{
		Active: s.Enabled || ua, UnderAttack: ua,
		RatePerMinute: s.RatePerMinute, Burst: s.Burst, MaxConcurrent: s.MaxConcurrent,
		MaxBodyBytes: s.MaxBodyBytes, BlockBots: s.BlockBots, RetryAfter: s.RetryAfterSeconds,
		Tarpit: time.Duration(s.TarpitMillis) * time.Millisecond,
	}
	if e.RetryAfter <= 0 {
		e.RetryAfter = l.ShieldRetryAfter
	}
	if e.Tarpit > l.ShieldTarpitMax {
		e.Tarpit = l.ShieldTarpitMax
	}
	if ua {
		e.RatePerMinute = clampPositive(e.RatePerMinute, l.ShieldAttackRate)
		e.Burst = clampPositive(e.Burst, l.ShieldAttackBurst)
		e.MaxConcurrent = clampPositive(e.MaxConcurrent, l.ShieldAttackConcurrent)
		e.BlockBots = true
	}
	if e.RatePerMinute > 0 && e.Burst <= 0 {
		e.Burst = max(1, e.RatePerMinute/6)
	}
	if e.RatePerMinute > 0 && e.Burst > e.RatePerMinute {
		e.Burst = e.RatePerMinute
	}
	return e
}

func clampPositive(v, ceiling int) int {
	if v <= 0 || v > ceiling {
		return ceiling
	}
	return v
}

// Validate checks the section.
func (s *Shield) Validate(l Limits) error {
	col := &collector{kind: KindShield}
	nonNeg := func(field string, v int) {
		if v < 0 {
			col.add(field, "must not be negative")
		}
	}
	nonNeg("rate_per_minute", s.RatePerMinute)
	nonNeg("burst", s.Burst)
	nonNeg("max_concurrent", s.MaxConcurrent)
	nonNeg("retry_after_seconds", s.RetryAfterSeconds)
	nonNeg("tarpit_ms", s.TarpitMillis)
	if s.MaxBodyBytes < 0 {
		col.add("max_body_bytes", "must not be negative")
	}
	if s.Burst > 0 && s.RatePerMinute == 0 {
		col.add("burst", "needs rate_per_minute")
	}
	if s.RetryAfterSeconds > l.ShieldMaxRetryAfter {
		col.add("retry_after_seconds", "must be at most %d", l.ShieldMaxRetryAfter)
	}
	if time.Duration(s.TarpitMillis)*time.Millisecond > l.ShieldTarpitMax {
		col.add("tarpit_ms", "must be at most %d", l.ShieldTarpitMax.Milliseconds())
	}
	if len(s.BotAgents) > l.ShieldMaxBotAgents {
		col.add("bot_agents", "at most %d entries", l.ShieldMaxBotAgents)
	}
	for i, a := range s.BotAgents {
		if strings.TrimSpace(a) == "" || len(a) > 128 || !SafeHeaderValue(a) {
			col.add(fmt.Sprintf("bot_agents[%d]", i), "must be 1 to 128 printable characters")
		}
	}
	if len(s.Allow) > l.MaxExemptEntries {
		col.add("allow", "at most %d entries", l.MaxExemptEntries)
	}
	for i, e := range s.Allow {
		if _, err := ParseIPOrPrefix(e); err != nil {
			col.add(fmt.Sprintf("allow[%d]", i), "must be an IP address or CIDR range")
		}
	}
	if s.UnderAttackUntil != nil && s.UnderAttackUntil.After(time.Now().Add(l.ShieldAttackMax)) {
		col.add("under_attack_until", "must be within %s from now", l.ShieldAttackMax)
	}
	if s.Enabled && s.RatePerMinute == 0 && s.MaxConcurrent == 0 && s.MaxBodyBytes == 0 && !s.BlockBots {
		col.add("", "turn on at least one protection")
	}
	return col.err()
}
