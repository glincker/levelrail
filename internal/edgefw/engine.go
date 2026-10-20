package edgefw

import (
	_ "embed"
	"net/netip"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

//go:embed ai_bots.txt
var shippedAIBots string

// DefaultAIBotAgents returns the shipped AI crawler list.
func DefaultAIBotAgents() []string { return trafficpolicy.ParseBotList(shippedAIBots) }

// Mode is how a bot list is enforced.
type Mode string

// Bot modes.
const (
	ModeOff       Mode = "off"
	ModeLog       Mode = "log"
	ModeChallenge Mode = "challenge"
	ModeBlock     Mode = "block"
)

func validMode(m Mode) bool {
	return m == ModeOff || m == ModeLog || m == ModeChallenge || m == ModeBlock
}

// Settings is the instance-wide firewall configuration beside the rules.
type Settings struct {
	BotMode         Mode       `json:"bot_mode"`
	AIBotMode       Mode       `json:"ai_bot_mode"`
	AIBotAgents     []string   `json:"ai_bot_agents"`
	BansEnabled     bool       `json:"bans_enabled"`
	AttackUntil     *time.Time `json:"attack_until,omitempty"`
	AttackChallenge bool       `json:"attack_challenge"`
}

// DefaultSettings leaves both bot lists off and automatic bans on.
func DefaultSettings() Settings {
	return Settings{BotMode: ModeOff, AIBotMode: ModeOff, BansEnabled: true, AIBotAgents: []string{}}
}

// AttackActive reports whether attack mode is on at now.
func (s Settings) AttackActive(now time.Time) bool {
	return s.AttackUntil != nil && now.Before(*s.AttackUntil)
}

// Validate checks the settings against the limits.
func (s *Settings) Validate(l Limits, attackMax time.Duration, now time.Time) error {
	v := &verrs{}
	if !validMode(s.BotMode) {
		v.add("bot_mode", "choose off, log, challenge or block")
	}
	if !validMode(s.AIBotMode) {
		v.add("ai_bot_mode", "choose off, log, challenge or block")
	}
	if len(s.AIBotAgents) > l.MaxBotAgents {
		v.add("ai_bot_agents", "at most %d entries", l.MaxBotAgents)
	}
	for i, a := range s.AIBotAgents {
		if strings.TrimSpace(a) == "" || len(a) > 128 || !trafficpolicy.SafeHeaderValue(a) {
			v.add("ai_bot_agents["+strconv.Itoa(i)+"]", "must be 1 to 128 printable characters")
		}
	}
	if s.AttackUntil != nil && s.AttackUntil.After(now.Add(attackMax)) {
		v.add("attack_until", "must be within %s from now", attackMax)
	}
	return v.err()
}

// Verdict is what Evaluate decided. Action is empty when the request goes
// on to the remaining checks.
type Verdict struct {
	Action     Action
	Reason     Reason
	Outcome    Outcome
	RuleID     string
	RuleName   string
	Status     int
	RetryAfter int
	Logged     bool
}

type snapshot struct {
	country bool
	global  []*compiledRule
	byHost  map[string][]*compiledRule
	limiter map[string]*rateLimiter
}

func (s *snapshot) rulesFor(host string) []*compiledRule {
	if r, ok := s.byHost[host]; ok {
		return r
	}
	return s.global
}

type botSet struct {
	bad, ai *trafficpolicy.BotMatcher
}

// Engine evaluates requests against the current rules and settings. All
// state it keeps is bounded; see Stats and Bans.
type Engine struct {
	lim      Limits
	snap     atomic.Pointer[snapshot]
	settings atomic.Pointer[Settings]
	bots     atomic.Pointer[botSet]
	Stats    *Stats
	Bans     *Bans
	Signer   *Signer
	title    atomic.Pointer[string]
	mu       sync.Mutex
	badCfg   trafficpolicy.Limits
}

// New builds an engine with empty rules and default settings.
func New(l Limits, shield trafficpolicy.Limits) *Engine {
	e := &Engine{lim: l, Stats: NewStats(l), Bans: NewBans(l), Signer: NewSigner(), badCfg: shield}
	e.snap.Store(&snapshot{byHost: map[string][]*compiledRule{}, limiter: map[string]*rateLimiter{}})
	e.SetSettings(DefaultSettings())
	t := "Security check"
	e.title.Store(&t)
	return e
}

var (
	defaultOnce   sync.Once
	defaultEngine *Engine
)

// Default is the process-wide engine the ingress handler reads; Caddy
// modules are built from JSON and cannot receive a live value any other way.
func Default() *Engine {
	defaultOnce.Do(func() {
		defaultEngine = New(LimitsFromEnv(os.Getenv), trafficpolicy.LimitsFromEnv(os.Getenv))
	})
	return defaultEngine
}

// Limits returns the limits the engine was built with.
func (e *Engine) Limits() Limits { return e.lim }

// SetTitle sets the brand name shown on the challenge page.
func (e *Engine) SetTitle(name string) {
	if strings.TrimSpace(name) != "" {
		e.title.Store(&name)
	}
}

// Title is the challenge page title.
func (e *Engine) Title() string { return *e.title.Load() }

// SetRules compiles and installs the rules; disabled rules are skipped.
func (e *Engine) SetRules(rules []Rule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	sorted := append([]Rule(nil), rules...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Position != sorted[j].Position {
			return sorted[i].Position < sorted[j].Position
		}
		return sorted[i].ID < sorted[j].ID
	})
	old := e.snap.Load()
	next := &snapshot{byHost: map[string][]*compiledRule{}, limiter: map[string]*rateLimiter{}}
	keep := make(map[string]bool, len(sorted))
	var scoped []*compiledRule
	hosts := map[string]bool{}
	for _, r := range sorted {
		keep[r.ID] = true
		if !r.Enabled {
			continue
		}
		c := compileRule(r)
		for _, cond := range r.Conditions {
			if cond.Field == FieldCountry {
				next.country = true
			}
		}
		if r.Action == ActionRateLimit {
			if l, ok := old.limiter[r.ID]; ok && l.same(r.RateCount, r.RateWindowSeconds) {
				next.limiter[r.ID] = l
			} else {
				next.limiter[r.ID] = newRateLimiter(r.RateCount, r.RateWindowSeconds, e.lim.MaxRateEntries)
			}
		}
		if r.Scope == "" {
			next.global = append(next.global, c)
		} else {
			hosts[r.Scope] = true
		}
		scoped = append(scoped, c)
	}
	for h := range hosts {
		for _, c := range scoped {
			if c.scope == "" || c.scope == h {
				next.byHost[h] = append(next.byHost[h], c)
			}
		}
	}
	e.snap.Store(next)
	e.Stats.PruneRules(keep)
}

// NeedsCountry reports whether any active rule reads the country, so the
// ingress only pays for a geo lookup when one does.
func (e *Engine) NeedsCountry() bool { return e.snap.Load().country }

// SetSettings installs the settings and rebuilds the bot matchers.
func (e *Engine) SetSettings(s Settings) {
	if s.AIBotAgents == nil {
		s.AIBotAgents = []string{}
	}
	e.settings.Store(&s)
	bad, _ := trafficpolicy.NewBotMatcher(e.badCfg, nil)
	agents := s.AIBotAgents
	if len(agents) == 0 {
		agents = DefaultAIBotAgents()
	}
	ai := trafficpolicy.NewBotMatcherFrom(agents)
	e.bots.Store(&botSet{bad: bad, ai: ai})
}

// CurrentSettings returns the installed settings.
func (e *Engine) CurrentSettings() Settings { return *e.settings.Load() }

func (e *Engine) passed(req *Request, now time.Time) bool {
	return e.Signer.PassValid(req.PassCookie, req.IP, now)
}

// Evaluate runs the ordered rules, then automatic bans, attack-mode
// challenge and the bot modes. It never allocates per rule.
func (e *Engine) Evaluate(req *Request, now time.Time) Verdict {
	snap := e.snap.Load()
	st := e.settings.Load()
	var v Verdict
	for _, r := range snap.rulesFor(req.Host) {
		if !r.matches(req) {
			continue
		}
		e.Stats.RuleMatched(r.id, now)
		switch r.action {
		case ActionAllow:
			return Verdict{Action: ActionAllow, Reason: ReasonRule, RuleID: r.id, RuleName: r.name}
		case ActionDeny:
			return Verdict{Action: ActionDeny, Reason: ReasonRule, Outcome: OutcomeDenied, RuleID: r.id, RuleName: r.name, Status: 403}
		case ActionChallenge:
			if !e.passed(req, now) {
				return Verdict{Action: ActionChallenge, Reason: ReasonRule, Outcome: OutcomeChallenged, RuleID: r.id, RuleName: r.name}
			}
		case ActionLog:
			v.Logged = true
		case ActionRateLimit:
			if ok, ra := snap.limiter[r.id].allow(req.IP, now); !ok {
				return Verdict{Action: ActionDeny, Reason: ReasonRateRule, Outcome: OutcomeRateLimited, RuleID: r.id, RuleName: r.name, Status: 429, RetryAfter: ra}
			}
		}
	}
	if st.BansEnabled && req.IP.IsValid() && e.Bans.Active(req.IP, now) {
		return Verdict{Action: ActionDeny, Reason: ReasonBan, Outcome: OutcomeDenied, Status: 403, Logged: v.Logged}
	}
	if st.AttackActive(now) && st.AttackChallenge && !e.passed(req, now) {
		return Verdict{Action: ActionChallenge, Reason: ReasonAttack, Outcome: OutcomeChallenged}
	}
	bots := e.bots.Load()
	if st.BotMode != ModeOff && bots.bad.Match(req.UserAgent) {
		if out, done := e.botVerdict(st.BotMode, ReasonBot, req, now); done {
			out.Logged = v.Logged
			return out
		}
		v.Logged = v.Logged || st.BotMode == ModeLog
	}
	if st.AIBotMode != ModeOff && bots.ai.Match(req.UserAgent) {
		if out, done := e.botVerdict(st.AIBotMode, ReasonAIBot, req, now); done {
			out.Logged = v.Logged
			return out
		}
		v.Logged = v.Logged || st.AIBotMode == ModeLog
	}
	return v
}

func (e *Engine) botVerdict(m Mode, reason Reason, req *Request, now time.Time) (Verdict, bool) {
	switch m {
	case ModeBlock:
		return Verdict{Action: ActionDeny, Reason: reason, Outcome: OutcomeDenied, Status: 403}, true
	case ModeChallenge:
		if !e.passed(req, now) {
			return Verdict{Action: ActionChallenge, Reason: reason, Outcome: OutcomeChallenged}, true
		}
	}
	return Verdict{}, false
}

// Record classifies a finished decision: it feeds the stats and, for denials
// and rate limits, counts a strike toward an automatic ban.
func (e *Engine) Record(req *Request, o Outcome, r Reason, now time.Time) {
	e.Stats.Observe(Observation{At: now, Outcome: o, Reason: r, IP: req.IP, UA: req.UserAgent, Path: req.Path, Host: req.Host})
	if (o == OutcomeDenied || o == OutcomeRateLimited) && r != ReasonBan && e.settings.Load().BansEnabled {
		e.Bans.Strike(req.IP, r.String(), now)
	}
}

// Issue creates a challenge token for req's client.
func (e *Engine) Issue(req *Request, now time.Time) string {
	return e.Signer.NewChallenge(req.IP, e.lim.ChallengeBits, e.lim.ChallengeTTL, now)
}

// Solve verifies a posted proof and returns the pass cookie value.
func (e *Engine) Solve(ip netip.Addr, token, counter string, now time.Time) (string, bool) {
	if !e.Signer.VerifyProof(token, counter, ip, now) {
		return "", false
	}
	return e.Signer.NewPass(ip, e.lim.PassTTL, now), true
}
