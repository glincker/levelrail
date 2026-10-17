package trafficpolicy

import (
	"net/netip"
	"strings"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func TestIPLimiter_TokenBucket(t *testing.T) {
	addr := netip.MustParseAddr("203.0.113.9")
	tests := []struct {
		name  string
		rate  int
		burst int
		steps []time.Duration
		want  []bool
	}{
		{"burst then deny", 60, 3, []time.Duration{0, 0, 0, 0}, []bool{true, true, true, false}},
		{"refills one per second", 60, 1, []time.Duration{0, 0, time.Second}, []bool{true, false, true}},
		{"slow rate refills slowly", 6, 1, []time.Duration{0, 5 * time.Second, 6 * time.Second}, []bool{true, false, true}},
		{"refill capped at burst", 60, 2, []time.Duration{0, 0, time.Hour, 0, 0}, []bool{true, true, true, true, false}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clk := &fakeClock{t: time.Unix(1000, 0)}
			l := NewIPLimiter(10, clk.now)
			for i, step := range tc.steps {
				clk.t = clk.t.Add(step)
				release, _, ok := l.Acquire(addr, tc.rate, tc.burst, 0)
				if ok != tc.want[i] {
					t.Fatalf("step %d: ok = %v, want %v", i, ok, tc.want[i])
				}
				if ok {
					release()
				}
			}
		})
	}
}

func TestIPLimiter_Concurrency(t *testing.T) {
	l := NewIPLimiter(10, nil)
	addr := netip.MustParseAddr("203.0.113.9")
	r1, _, ok1 := l.Acquire(addr, 0, 0, 2)
	r2, _, ok2 := l.Acquire(addr, 0, 0, 2)
	_, reason, ok3 := l.Acquire(addr, 0, 0, 2)
	if !ok1 || !ok2 || ok3 || reason != ShieldConcurrency {
		t.Fatalf("got %v %v %v %v", ok1, ok2, ok3, reason)
	}
	r1()
	r1()
	if _, _, ok := l.Acquire(addr, 0, 0, 2); !ok {
		t.Fatal("slot not freed after release")
	}
	r2()
}

func TestIPLimiter_LRUCap(t *testing.T) {
	l := NewIPLimiter(3, nil)
	for i := 0; i < 50; i++ {
		a := netip.AddrFrom4([4]byte{10, 0, 0, byte(i)})
		if r, _, ok := l.Acquire(a, 60, 5, 0); ok {
			r()
		}
		if l.Len() > 3 {
			t.Fatalf("tracked %d addresses, cap is 3", l.Len())
		}
	}
	if l.Len() != 3 {
		t.Fatalf("Len = %d, want 3", l.Len())
	}
}

func TestShieldEffective_UnderAttack(t *testing.T) {
	l := DefaultLimits()
	now := time.Unix(5000, 0)
	until := now.Add(time.Minute)
	tests := []struct {
		name     string
		s        Shield
		at       time.Time
		active   bool
		rate     int
		conc     int
		bots     bool
		attacked bool
	}{
		{"disabled and calm", Shield{RatePerMinute: 600}, now, false, 600, 0, false, false},
		{"enabled", Shield{Enabled: true, RatePerMinute: 600}, now, true, 600, 0, false, false},
		{"under attack clamps loose rate", Shield{Enabled: true, RatePerMinute: 6000, UnderAttackUntil: &until}, now, true, l.ShieldAttackRate, l.ShieldAttackConcurrent, true, true},
		{"under attack keeps tighter rate", Shield{Enabled: true, RatePerMinute: 10, UnderAttackUntil: &until}, now, true, 10, l.ShieldAttackConcurrent, true, true},
		{"under attack works with nothing configured", Shield{UnderAttackUntil: &until}, now, true, l.ShieldAttackRate, l.ShieldAttackConcurrent, true, true},
		{"expired reverts", Shield{Enabled: true, RatePerMinute: 6000, UnderAttackUntil: &until}, until.Add(time.Second), true, 6000, 0, false, false},
		{"expired and disabled is inert", Shield{UnderAttackUntil: &until}, until, false, 0, 0, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := tc.s.Effective(tc.at, l)
			if e.Active != tc.active || e.RatePerMinute != tc.rate || e.MaxConcurrent != tc.conc || e.BlockBots != tc.bots || e.UnderAttack != tc.attacked {
				t.Fatalf("got %+v", e)
			}
		})
	}
}

func TestShieldEffective_BurstDefaults(t *testing.T) {
	l := DefaultLimits()
	tests := []struct {
		rate, burst, want int
	}{{600, 0, 100}, {3, 0, 1}, {60, 500, 60}, {60, 7, 7}}
	for _, tc := range tests {
		e := Shield{Enabled: true, RatePerMinute: tc.rate, Burst: tc.burst}.Effective(time.Now(), l)
		if e.Burst != tc.want {
			t.Errorf("rate %d burst %d: got %d, want %d", tc.rate, tc.burst, e.Burst, tc.want)
		}
	}
}

func TestShieldEffective_TarpitAndRetryCaps(t *testing.T) {
	l := DefaultLimits()
	e := Shield{Enabled: true, MaxConcurrent: 1, TarpitMillis: 600000}.Effective(time.Now(), l)
	if e.Tarpit != l.ShieldTarpitMax || e.RetryAfter != l.ShieldRetryAfter {
		t.Fatalf("got tarpit %v retry %d", e.Tarpit, e.RetryAfter)
	}
}

func TestShieldValidate(t *testing.T) {
	l := DefaultLimits()
	far := time.Now().Add(l.ShieldAttackMax * 2)
	tests := []struct {
		name  string
		s     Shield
		field string
	}{
		{"ok", Shield{Enabled: true, RatePerMinute: 60, Burst: 10, Allow: []string{"10.0.0.0/8", "203.0.113.4"}}, ""},
		{"nothing on", Shield{Enabled: true}, "turn on"},
		{"disabled is fine", Shield{}, ""},
		{"negative rate", Shield{Enabled: true, RatePerMinute: -1}, "rate_per_minute"},
		{"burst without rate", Shield{Enabled: true, MaxConcurrent: 2, Burst: 4}, "burst"},
		{"bad cidr", Shield{Enabled: true, BlockBots: true, Allow: []string{"nope"}}, "allow[0]"},
		{"empty bot agent", Shield{Enabled: true, BlockBots: true, BotAgents: []string{" "}}, "bot_agents[0]"},
		{"retry too large", Shield{Enabled: true, BlockBots: true, RetryAfterSeconds: l.ShieldMaxRetryAfter + 1}, "retry_after_seconds"},
		{"tarpit too long", Shield{Enabled: true, BlockBots: true, TarpitMillis: int(l.ShieldTarpitMax.Milliseconds()) + 1}, "tarpit_ms"},
		{"attack too long", Shield{UnderAttackUntil: &far}, "under_attack_until"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.s.Validate(l)
			if tc.field == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("error = %v, want mention of %q", err, tc.field)
			}
		})
	}
}

func TestBotMatcher(t *testing.T) {
	m, err := NewBotMatcher(DefaultLimits(), []string{"  CustomCrawler "})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		ua   string
		want bool
	}{
		{"Mozilla/5.0 (compatible; AhrefsBot/7.0; +http://ahrefs.com/robot/)", true},
		{"Scrapy/2.11 (+https://scrapy.org)", true},
		{"MyCustomCrawler/1.0", true},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Safari/605.1.15", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := m.Match(tc.ua); got != tc.want {
			t.Errorf("Match(%q) = %v, want %v", tc.ua, got, tc.want)
		}
	}
	if len(DefaultBotAgents()) < 20 {
		t.Fatal("shipped bot list is unexpectedly short")
	}
}

func TestParseBotList(t *testing.T) {
	got := ParseBotList("# c\n\n  FooBot \nbar\n")
	if len(got) != 2 || got[0] != "foobot" || got[1] != "bar" {
		t.Fatalf("got %v", got)
	}
}

func TestPolicyDecodeShield(t *testing.T) {
	var p Policy
	if err := p.Decode(KindShield, []byte(`{"enabled":true,"rate_per_minute":30}`)); err != nil {
		t.Fatal(err)
	}
	if p.Shield == nil || p.Shield.RatePerMinute != 30 || p.Empty() {
		t.Fatalf("got %+v", p.Shield)
	}
}
