package nodeinsight

import (
	"strings"
	"testing"
	"time"
)

func at(h, m int) time.Time { return time.Date(2026, 1, 2, h, m, 0, 0, time.UTC) }

func TestParseWindowAndOpen(t *testing.T) {
	w, err := ParseWindow("02:00-05:00")
	if err != nil || !w.Open(at(2, 0)) || !w.Open(at(4, 59)) || w.Open(at(5, 0)) || w.Open(at(12, 0)) {
		t.Fatalf("plain window wrong: %v", err)
	}
	wrap, _ := ParseWindow("22:00-03:00")
	if !wrap.Open(at(23, 30)) || !wrap.Open(at(1, 0)) || wrap.Open(at(12, 0)) {
		t.Fatal("wrapping window wrong")
	}
	anyTime, err := ParseWindow("")
	if err != nil || !anyTime.Open(at(12, 0)) {
		t.Fatal("empty window must be any time")
	}
	for _, bad := range []string{"2-5", "02:00", "25:00-03:00", "02:00-02:00", "aa:bb-cc:dd"} {
		if _, err := ParseWindow(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestChannelAllows(t *testing.T) {
	if !ChannelAllows(ChannelStable, "v1.2.0", "v2.0.0") {
		t.Fatal("stable allows any")
	}
	if !ChannelAllows(ChannelHoldMinor, "v1.2.0", "v1.2.5") || ChannelAllows(ChannelHoldMinor, "v1.2.0", "v1.3.0") || ChannelAllows(ChannelHoldMinor, "v1.9.0", "v2.0.0") {
		t.Fatal("hold-minor only allows patch bumps")
	}
	if !ChannelAllows(ChannelHoldMinor, "dev", "v1.3.0") {
		t.Fatal("unparseable versions must not block")
	}
}

func TestUpdatePolicyEvaluate(t *testing.T) {
	base := UpdatePolicy{Auto: true, Channel: ChannelStable}
	if ok, why := base.Evaluate("v1.0.0", "v1.0.1", at(12, 0)); !ok || why != "" {
		t.Fatalf("open policy should apply: %q", why)
	}
	cases := map[string]UpdatePolicy{
		"automatic updates are off": {Channel: ChannelStable},
		"on hold":                   {Auto: true, Held: true},
		"hold-minor":                {Auto: true, Channel: ChannelHoldMinor},
		"maintenance window":        {Auto: true, WindowText: "02:00-05:00"},
	}
	for want, p := range cases {
		target := "v1.1.0"
		ok, why := p.Evaluate("v1.0.0", target, at(12, 0))
		if ok || !strings.Contains(why, want) {
			t.Errorf("%s: got apply=%v why=%q", want, ok, why)
		}
		p.Override = true
		if ok, _ := p.Evaluate("v1.0.0", target, at(12, 0)); !ok {
			t.Errorf("%s: override must apply", want)
		}
	}
}
