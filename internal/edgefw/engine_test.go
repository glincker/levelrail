package edgefw

import (
	"crypto/sha256"
	"errors"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

var t0 = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

func testIP(i int) netip.Addr {
	return netip.AddrFrom4([4]byte{10, byte(i >> 8), byte(i), 1}) //nolint:gosec // test addresses, i stays below 65536
}

func newEngine() *Engine {
	return New(DefaultLimits(), trafficpolicy.DefaultLimits())
}

func req(path, ua, ip string) *Request {
	return &Request{Method: "GET", Host: "app.example.com", Path: path, UserAgent: ua, Header: http.Header{}, IP: netip.MustParseAddr(ip)}
}

func rule(id string, pos int, action Action, conds ...Condition) Rule {
	return Rule{ID: id, Name: id, Enabled: true, Position: pos, Action: action, Conditions: conds}
}

func cond(f Field, o Op, v string) Condition { return Condition{Field: f, Op: o, Value: v} }

func TestMatchTable(t *testing.T) {
	r := req("/admin/users", "Mozilla/5.0 SemrushBot", "10.1.2.3")
	r.Header.Set("X-Env", "Prod")
	r.RawQuery = "a=1&debug=yes&b"
	r.Country = "DE"
	tests := []struct {
		name string
		c    Condition
		want bool
	}{
		{"path equals", cond(FieldPath, OpEquals, "/admin/users"), true},
		{"path equals miss", cond(FieldPath, OpEquals, "/admin"), false},
		{"path prefix", cond(FieldPath, OpStartsWith, "/admin"), true},
		{"path contains", cond(FieldPath, OpContains, "min/u"), true},
		{"path glob", cond(FieldPath, OpGlob, "/admin/*"), true},
		{"path glob question", cond(FieldPath, OpGlob, "/admin/user?"), true},
		{"path glob miss", cond(FieldPath, OpGlob, "/api/*"), false},
		{"path is case sensitive", cond(FieldPath, OpEquals, "/ADMIN/users"), false},
		{"host glob", cond(FieldHost, OpGlob, "*.example.com"), true},
		{"host equals folded", cond(FieldHost, OpEquals, "APP.example.com"), true},
		{"method", cond(FieldMethod, OpEquals, "GET"), true},
		{"method miss", cond(FieldMethod, OpEquals, "POST"), false},
		{"ua contains folded", cond(FieldUserAgent, OpContains, "semrushbot"), true},
		{"ua prefix", cond(FieldUserAgent, OpStartsWith, "mozilla"), true},
		{"ip in cidr", cond(FieldIP, OpInCIDR, "10.1.0.0/16"), true},
		{"ip outside", cond(FieldIP, OpInCIDR, "10.2.0.0/16"), false},
		{"ip single", cond(FieldIP, OpInCIDR, "10.1.2.3"), true},
		{"country", cond(FieldCountry, OpEquals, "DE"), true},
		{"country miss", cond(FieldCountry, OpEquals, "FR"), false},
		{"header equals folded", Condition{Field: FieldHeader, Op: OpEquals, Name: "x-env", Value: "prod"}, true},
		{"header exists", Condition{Field: FieldHeader, Op: OpExists, Name: "x-env"}, true},
		{"header missing", Condition{Field: FieldHeader, Op: OpExists, Name: "x-none"}, false},
		{"query equals", Condition{Field: FieldQuery, Op: OpEquals, Name: "debug", Value: "yes"}, true},
		{"query exists bare", Condition{Field: FieldQuery, Op: OpExists, Name: "b"}, true},
		{"query miss", Condition{Field: FieldQuery, Op: OpExists, Name: "zzz"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := compileCondition(tt.c)
			if got := m.match(r); got != tt.want {
				t.Fatalf("match = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGlobEdgeCases(t *testing.T) {
	tests := []struct {
		p, s string
		want bool
	}{
		{"*", "", true}, {"*", "abc", true}, {"a*c", "abbbc", true}, {"a*c", "abbbd", false},
		{"a**b", "ab", true}, {"?", "", false}, {"/x/*/y", "/x/1/2/y", true}, {"*.php", "/a/b.php", true},
		{"abc", "abcd", false}, {"*abc", "xxabc", true}, {"*abc", "xxabd", false},
	}
	for _, tt := range tests {
		if got := globMatch(tt.p, tt.s, false); got != tt.want {
			t.Errorf("glob(%q,%q) = %v, want %v", tt.p, tt.s, got, tt.want)
		}
	}
}

func TestEvaluateOrderAndTerminalActions(t *testing.T) {
	e := newEngine()
	e.SetRules([]Rule{
		rule("deny-admin", 20, ActionDeny, cond(FieldPath, OpStartsWith, "/admin")),
		rule("allow-office", 10, ActionAllow, cond(FieldIP, OpInCIDR, "192.0.2.0/24")),
		rule("log-all", 5, ActionLog, cond(FieldPath, OpStartsWith, "/")),
	})
	tests := []struct {
		name       string
		r          *Request
		wantAction Action
		wantRule   string
		wantLogged bool
	}{
		{"allow before deny", req("/admin", "x", "192.0.2.9"), ActionAllow, "allow-office", false},
		{"deny after log", req("/admin/x", "x", "198.51.100.1"), ActionDeny, "deny-admin", false},
		{"log continues", req("/home", "x", "198.51.100.1"), "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := e.Evaluate(tt.r, t0)
			if v.Action != tt.wantAction || v.RuleID != tt.wantRule || v.Logged != tt.wantLogged {
				t.Fatalf("verdict = %+v", v)
			}
		})
	}
}

func TestDisabledAndScopedRules(t *testing.T) {
	e := newEngine()
	off := rule("off", 1, ActionDeny, cond(FieldPath, OpStartsWith, "/"))
	off.Enabled = false
	scoped := rule("scoped", 2, ActionDeny, cond(FieldPath, OpStartsWith, "/s"))
	scoped.Scope = "other.example.com"
	e.SetRules([]Rule{off, scoped})
	if v := e.Evaluate(req("/s", "x", "10.0.0.1"), t0); v.Action != "" {
		t.Fatalf("disabled or other-host rule matched: %+v", v)
	}
	r := req("/s", "x", "10.0.0.1")
	r.Host = "other.example.com"
	if v := e.Evaluate(r, t0); v.Action != ActionDeny {
		t.Fatalf("scoped rule should match its host: %+v", v)
	}
}

func TestRateLimitRuleWindow(t *testing.T) {
	e := newEngine()
	rl := rule("rl", 1, ActionRateLimit, cond(FieldPath, OpStartsWith, "/api"))
	rl.RateCount, rl.RateWindowSeconds = 3, 60
	e.SetRules([]Rule{rl})
	r := req("/api/x", "x", "203.0.113.5")
	for i := 0; i < 3; i++ {
		if v := e.Evaluate(r, t0.Add(time.Duration(i)*time.Second)); v.Action != "" {
			t.Fatalf("request %d limited early: %+v", i, v)
		}
	}
	v := e.Evaluate(r, t0.Add(10*time.Second))
	if v.Action != ActionDeny || v.Status != 429 || v.Outcome != OutcomeRateLimited || v.RetryAfter != 51 {
		t.Fatalf("fourth request = %+v", v)
	}
	if v := e.Evaluate(req("/api/x", "x", "203.0.113.6"), t0.Add(10*time.Second)); v.Action != "" {
		t.Fatalf("other client limited: %+v", v)
	}
	if v := e.Evaluate(r, t0.Add(61*time.Second)); v.Action != "" {
		t.Fatalf("window did not reset: %+v", v)
	}
}

func TestRateLimiterStateSurvivesReloadAndIsBounded(t *testing.T) {
	e := newEngine()
	rl := rule("rl", 1, ActionRateLimit, cond(FieldPath, OpStartsWith, "/"))
	rl.RateCount, rl.RateWindowSeconds = 1, 60
	e.SetRules([]Rule{rl})
	r := req("/", "x", "203.0.113.5")
	e.Evaluate(r, t0)
	e.SetRules([]Rule{rl})
	if v := e.Evaluate(r, t0.Add(time.Second)); v.Action != ActionDeny {
		t.Fatalf("limiter state lost on reload: %+v", v)
	}
	l := newRateLimiter(1, 60, 100)
	for i := 0; i < 1000; i++ {
		ip := testIP(i)
		l.allow(ip, t0)
	}
	if n := l.size(); n > 100 {
		t.Fatalf("limiter grew to %d, cap 100", n)
	}
}

func TestBanMath(t *testing.T) {
	l := DefaultLimits()
	l.BanStrikes, l.BanWindow, l.BanDuration = 3, time.Minute, 10*time.Minute
	b := NewBans(l)
	ip := netip.MustParseAddr("198.51.100.7")
	if b.Strike(ip, "rule", t0) || b.Strike(ip, "rule", t0.Add(10*time.Second)) {
		t.Fatal("banned before threshold")
	}
	if !b.Strike(ip, "rule", t0.Add(20*time.Second)) {
		t.Fatal("third strike in window should ban")
	}
	if !b.Active(ip, t0.Add(5*time.Minute)) {
		t.Fatal("ban should be active")
	}
	if b.Active(ip, t0.Add(31*time.Second+10*time.Minute)) {
		t.Fatal("ban should have expired")
	}
	other := netip.MustParseAddr("198.51.100.8")
	b.Strike(other, "rule", t0)
	b.Strike(other, "rule", t0.Add(2*time.Minute))
	if b.Strike(other, "rule", t0.Add(4*time.Minute)) {
		t.Fatal("strikes outside the window must not accumulate")
	}
}

func TestBanLiftAndCap(t *testing.T) {
	l := DefaultLimits()
	l.MaxBans = 5
	b := NewBans(l)
	for i := 0; i < 20; i++ {
		b.Add(netip.AddrFrom4([4]byte{10, 0, 0, byte(i)}), "x", time.Duration(i+1)*time.Minute, t0)
	}
	if n := len(b.List(t0)); n > 5 {
		t.Fatalf("bans grew to %d, cap 5", n)
	}
	ip := netip.AddrFrom4([4]byte{10, 0, 0, 19})
	if !b.Lift(ip) || b.Active(ip, t0) {
		t.Fatal("lift did not remove the ban")
	}
}

func TestEngineAutoBanAndAllowBypass(t *testing.T) {
	e := newEngine()
	lim := DefaultLimits()
	lim.BanStrikes = 2
	e.Bans.SetLimits(lim)
	e.SetRules([]Rule{
		rule("allow", 1, ActionAllow, cond(FieldIP, OpInCIDR, "192.0.2.1")),
		rule("deny", 2, ActionDeny, cond(FieldPath, OpStartsWith, "/blocked")),
	})
	bad := req("/blocked", "x", "198.51.100.9")
	for i := 0; i < 2; i++ {
		v := e.Evaluate(bad, t0)
		e.Record(bad, v.Outcome, v.Reason, t0)
	}
	clean := req("/fine", "x", "198.51.100.9")
	if v := e.Evaluate(clean, t0.Add(time.Second)); v.Reason != ReasonBan {
		t.Fatalf("banned client should be denied everywhere: %+v", v)
	}
	trusted := req("/blocked", "x", "192.0.2.1")
	if v := e.Evaluate(trusted, t0); v.Action != ActionAllow {
		t.Fatalf("allow rule must win: %+v", v)
	}
}

func TestBotModes(t *testing.T) {
	tests := []struct {
		mode       Mode
		wantAction Action
		wantLogged bool
	}{
		{ModeOff, "", false}, {ModeLog, "", true}, {ModeChallenge, ActionChallenge, false}, {ModeBlock, ActionDeny, false},
	}
	for _, tt := range tests {
		e := newEngine()
		s := DefaultSettings()
		s.AIBotMode = tt.mode
		e.SetSettings(s)
		v := e.Evaluate(req("/", "Mozilla GPTBot/1.0", "10.0.0.1"), t0)
		if v.Action != tt.wantAction || v.Logged != tt.wantLogged {
			t.Errorf("mode %s: %+v", tt.mode, v)
		}
		if v := e.Evaluate(req("/", "Mozilla Firefox", "10.0.0.1"), t0); v.Action != "" {
			t.Errorf("mode %s blocked a browser: %+v", tt.mode, v)
		}
	}
}

func TestCustomAIBotListReplacesDefault(t *testing.T) {
	e := newEngine()
	s := DefaultSettings()
	s.AIBotMode = ModeBlock
	s.AIBotAgents = []string{"mycrawler"}
	e.SetSettings(s)
	if v := e.Evaluate(req("/", "GPTBot", "10.0.0.1"), t0); v.Action != "" {
		t.Fatal("custom list should replace the shipped list")
	}
	if v := e.Evaluate(req("/", "MyCrawler/2", "10.0.0.1"), t0); v.Action != ActionDeny {
		t.Fatal("custom entry should match")
	}
}

func TestAttackChallengeAndPassCookie(t *testing.T) {
	e := newEngine()
	until := t0.Add(time.Hour)
	s := DefaultSettings()
	s.AttackUntil, s.AttackChallenge = &until, true
	e.SetSettings(s)
	r := req("/", "x", "198.51.100.20")
	if v := e.Evaluate(r, t0); v.Action != ActionChallenge || v.Reason != ReasonAttack {
		t.Fatalf("attack mode should challenge: %+v", v)
	}
	r.PassCookie = e.Signer.NewPass(r.IP, time.Minute, t0)
	if v := e.Evaluate(r, t0); v.Action != "" {
		t.Fatalf("valid pass should skip the challenge: %+v", v)
	}
	if v := e.Evaluate(r, t0.Add(2*time.Hour)); v.Action != "" {
		t.Fatalf("attack mode should lapse: %+v", v)
	}
}

func solve(token string, bits int) string {
	for n := 0; ; n++ {
		c := strconv.Itoa(n)
		if LeadingZeroBits(sha256.Sum256([]byte(token+":"+c))) >= bits {
			return c
		}
	}
}

func TestChallengeSigning(t *testing.T) {
	s := NewSigner()
	ip := netip.MustParseAddr("198.51.100.20")
	tok := s.NewChallenge(ip, 8, time.Minute, t0)
	good := solve(tok, 8)
	tests := []struct {
		name    string
		token   string
		counter string
		ip      netip.Addr
		now     time.Time
		want    bool
	}{
		{"valid", tok, good, ip, t0, true},
		{"same /24", tok, good, netip.MustParseAddr("198.51.100.99"), t0, true},
		{"other network", tok, good, netip.MustParseAddr("203.0.113.1"), t0, false},
		{"expired", tok, good, ip, t0.Add(2 * time.Minute), false},
		{"wrong counter", tok, "x", ip, t0, false},
		{"empty counter", tok, "", ip, t0, false},
		{"tampered body", "A" + tok[1:], good, ip, t0, false},
		{"tampered mac", tok[:len(tok)-2] + "AA", good, ip, t0, false},
		{"garbage", "nope", "1", ip, t0, false},
		{"pass cookie is not a token", s.NewPass(ip, time.Minute, t0), good, ip, t0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.VerifyProof(tt.token, tt.counter, tt.ip, tt.now); got != tt.want {
				t.Fatalf("VerifyProof = %v, want %v", got, tt.want)
			}
		})
	}
	if bad := s.VerifyProof(tok, "0", ip, t0); bad && LeadingZeroBits(sha256.Sum256([]byte(tok+":0"))) < 8 {
		t.Fatal("accepted a proof below the difficulty")
	}
}

func TestPassCookie(t *testing.T) {
	s := NewSigner()
	ip := netip.MustParseAddr("2001:db8:1:2::5")
	c := s.NewPass(ip, time.Minute, t0)
	if !s.PassValid(c, ip, t0) || !s.PassValid(c, netip.MustParseAddr("2001:db8:1:2::99"), t0) {
		t.Fatal("pass should hold inside the /64")
	}
	if s.PassValid(c, netip.MustParseAddr("2001:db8:1:3::5"), t0) || s.PassValid(c, ip, t0.Add(time.Hour)) || s.PassValid("", ip, t0) {
		t.Fatal("pass accepted outside its binding or lifetime")
	}
	other := NewSigner()
	if other.PassValid(c, ip, t0) {
		t.Fatal("pass signed by another key accepted")
	}
	s.SetKey([]byte(strings.Repeat("k", 32)))
	if s.PassValid(c, ip, t0) {
		t.Fatal("key rotation should invalidate old passes")
	}
}

func TestEngineSolveAndPage(t *testing.T) {
	e := newEngine()
	e.lim.ChallengeBits = 8
	r := req("/", "x", "198.51.100.20")
	tok := e.Issue(r, t0)
	cookie, ok := e.Solve(r.IP, tok, solve(tok, 8), t0)
	if !ok || !e.Signer.PassValid(cookie, r.IP, t0) {
		t.Fatal("solve did not yield a valid pass")
	}
	page, csp := ChallengePage("Brand <x>", tok, 8)
	if !strings.Contains(string(page), "Brand &lt;x&gt;") || strings.Contains(string(page), "http://") || strings.Contains(string(page), "https://") {
		t.Fatal("page must escape the title and make no external references")
	}
	if !strings.Contains(csp, "script-src 'nonce-") || !strings.Contains(csp, "default-src 'none'") {
		t.Fatalf("csp = %q", csp)
	}
}

func TestStatsBoundsAndSeries(t *testing.T) {
	l := DefaultLimits()
	l.MaxStatScopes, l.TopCapacity = 3, 4
	s := NewStats(l)
	for i := 0; i < 5000; i++ {
		s.Observe(Observation{
			At: t0, Outcome: Outcome(i % int(OutcomeCount)), Reason: ReasonRule,
			IP: testIP(i), UA: "ua-" + strconv.Itoa(i),
			Path: "/p/" + strconv.Itoa(i) + "/x", Host: "h" + strconv.Itoa(i%50) + ".example.com",
		})
	}
	if n := len(s.hosts); n > 3 {
		t.Fatalf("host scopes = %d, cap 3", n)
	}
	for _, d := range []Dimension{DimIP, DimDeniedIP, DimUA, DimPath, DimHost} {
		if rows := s.Top("", d, t0, 100); len(rows) > 4 {
			t.Fatalf("%s top rows = %d, cap 4", d, len(rows))
		}
	}
	tot := s.Totals("", t0.Add(-time.Hour), t0.Add(time.Minute))
	var sum uint64
	for _, n := range tot {
		sum += n
	}
	if sum != 5000 {
		t.Fatalf("totals = %d, want 5000", sum)
	}
	if tot[OutcomeDenied] != 1000 {
		t.Fatalf("denied = %d", tot[OutcomeDenied])
	}
	if got := s.Reasons("", t0.Add(-time.Hour), t0); len(got) != 1 || got[0].Reason != "rule" {
		t.Fatalf("reasons = %+v", got)
	}
}

func TestStatsRetentionAndRuleHits(t *testing.T) {
	s := NewStats(DefaultLimits())
	s.Observe(Observation{At: t0, Outcome: OutcomeAllowed, Host: "a"})
	later := t0.Add(25 * time.Hour)
	s.Observe(Observation{At: later, Outcome: OutcomeAllowed, Host: "a"})
	if tot := s.Totals("", t0.Add(-time.Hour), later); tot[OutcomeAllowed] != 1 {
		t.Fatalf("old bucket should be overwritten after 24h: %d", tot[OutcomeAllowed])
	}
	for i := 0; i < 7; i++ {
		s.RuleMatched("r1", later.Add(-time.Duration(i)*time.Minute))
	}
	h := s.RuleHits("r1", later)
	var sp uint32
	for _, v := range h.Spark {
		sp += v
	}
	if h.Total != 7 || sp != 7 {
		t.Fatalf("hits = %+v", h)
	}
	s.PruneRules(map[string]bool{})
	if h := s.RuleHits("r1", later); h.Total != 0 {
		t.Fatal("pruned rule kept counters")
	}
}

func TestValidateRule(t *testing.T) {
	ok := Rule{Name: "n", Enabled: true, Action: ActionDeny, Conditions: []Condition{cond(FieldPath, OpStartsWith, "/x")}}
	tests := []struct {
		name   string
		mutate func(*Rule)
		field  string
	}{
		{"valid", func(*Rule) {}, ""},
		{"no name", func(r *Rule) { r.Name = "" }, "name"},
		{"no conditions", func(r *Rule) { r.Conditions = nil }, "conditions"},
		{"bad action", func(r *Rule) { r.Action = "nuke" }, "action"},
		{"rate on deny", func(r *Rule) { r.RateCount = 5 }, "action"},
		{"rate missing", func(r *Rule) { r.Action = ActionRateLimit }, "rate_count"},
		{"op not for field", func(r *Rule) { r.Conditions[0].Op = OpInCIDR }, "conditions[0].op"},
		{"path needs slash", func(r *Rule) { r.Conditions[0].Value = "x" }, "conditions[0].value"},
		{"bad cidr", func(r *Rule) { r.Conditions = []Condition{cond(FieldIP, OpInCIDR, "nope")} }, "conditions[0].value"},
		{"header needs name", func(r *Rule) { r.Conditions = []Condition{cond(FieldHeader, OpEquals, "x")} }, "conditions[0].name"},
		{"bad country", func(r *Rule) { r.Conditions = []Condition{cond(FieldCountry, OpEquals, "ZZZZ")} }, "conditions[0].value"},
		{"too many wildcards", func(r *Rule) { r.Conditions[0] = cond(FieldPath, OpGlob, strings.Repeat("*a", 9)) }, "conditions[0].value"},
		{"bad scope", func(r *Rule) { r.Scope = "Bad Host" }, "scope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := ok
			r.Conditions = append([]Condition(nil), ok.Conditions...)
			tt.mutate(&r)
			Normalize(&r)
			err := ValidateRule(&r)
			if tt.field == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var ve *ValidationError
			isVE := errors.As(err, &ve)
			if !isVE || ve.Fields[0].Field != tt.field {
				t.Fatalf("error = %v, want field %s", err, tt.field)
			}
		})
	}
}

func TestIPRuleViews(t *testing.T) {
	r, err := IPRule(ActionDeny, " 203.0.113.0/24 ", "", "scanner")
	if err != nil {
		t.Fatal(err)
	}
	if a, ok := r.IPList(); !ok || a != ActionDeny {
		t.Fatalf("ip rule not in the block list view: %+v", r)
	}
	if _, err := IPRule(ActionDeny, "not-an-ip", "", ""); err == nil {
		t.Fatal("expected error")
	}
	other := rule("x", 1, ActionDeny, cond(FieldPath, OpEquals, "/"))
	if _, ok := other.IPList(); ok {
		t.Fatal("path rule must not be in the ip list")
	}
}

func TestSettingsValidate(t *testing.T) {
	s := DefaultSettings()
	s.BotMode = "weird"
	s.AIBotAgents = []string{""}
	far := t0.Add(1000 * time.Hour)
	s.AttackUntil = &far
	err := s.Validate(DefaultLimits(), 24*time.Hour, t0)
	var ve *ValidationError
	ok := errors.As(err, &ve)
	if !ok || len(ve.Fields) != 3 {
		t.Fatalf("err = %v", err)
	}
}

func TestLimitsFromEnv(t *testing.T) {
	env := map[string]string{EnvBanStrikes: "7", EnvBanDuration: "2h", EnvChallengeBits: "99", EnvMaxRules: "-1", EnvPassTTL: "junk"}
	l := LimitsFromEnv(func(k string) string { return env[k] })
	d := DefaultLimits()
	if l.BanStrikes != 7 || l.BanDuration != 2*time.Hour || l.ChallengeBits != 28 || l.MaxRules != d.MaxRules || l.PassTTL != d.PassTTL {
		t.Fatalf("limits = %+v", l)
	}
}

func BenchmarkEvaluate(b *testing.B) {
	e := newEngine()
	var rules []Rule
	for i := 0; i < 100; i++ {
		rules = append(rules, rule("r"+strconv.Itoa(i), i, ActionDeny, cond(FieldPath, OpStartsWith, "/nomatch"+strconv.Itoa(i))))
	}
	e.SetRules(rules)
	r := req("/index", "Mozilla/5.0", "10.0.0.1")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e.Evaluate(r, t0)
	}
}
