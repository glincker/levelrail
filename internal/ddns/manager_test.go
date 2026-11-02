package ddns

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type memSecrets map[string]string

func (m memSecrets) SetValue(_ context.Context, svc, k, v string) error { m[svc+"/"+k] = v; return nil }
func (m memSecrets) Resolve(_ context.Context, svc, k string) (string, error) {
	return m[svc+"/"+k], nil
}
func (m memSecrets) Exists(_ context.Context, svc, k string) (bool, error) {
	_, ok := m[svc+"/"+k]
	return ok, nil
}
func (m memSecrets) DeleteValue(_ context.Context, svc, k string) error {
	delete(m, svc+"/"+k)
	return nil
}

func testEnv(extra map[string]string) func(string) string {
	base := map[string]string{EnvAllowPrivate: "on", EnvProbeURLs: "https://127.0.0.1:1/ip", EnvProbeQuorum: "1", EnvInterval: "1h"}
	for k, v := range extra {
		base[k] = v
	}
	return func(k string) string { return base[k] }
}

func ptr[T any](v T) *T { return &v }

func TestManagerStoredOverridesEnvAndAppliesLive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(ctx, memSecrets{}, testEnv(map[string]string{EnvEnabled: "off", EnvInterval: "10m"}), nil, Hooks{})
	res, err := m.Get(ctx)
	if err != nil || res.Source != SourceDefault || res.Enabled || res.IntervalSecs != 600 {
		t.Fatalf("default view = %+v, %v", res, err)
	}
	secretURL := "https://127.0.0.1/update?key=" + "topsecret" + "&ip={ip}"
	res, err = m.Apply(ctx, Update{Enabled: ptr(true), Target: ptr(secretURL), IntervalSecs: ptr(120)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceStored || !res.Enabled || res.IntervalSecs != 120 || !res.TargetConfigured || !res.Running {
		t.Fatalf("stored view = %+v", res)
	}
	raw, _ := json.Marshal(res)
	if strings.Contains(string(raw), "topsecret") || strings.Contains(string(raw), "update?") {
		t.Fatalf("target leaked: %s", raw)
	}
	if res.Host != "127.0.0.1" {
		t.Errorf("host = %q", res.Host)
	}
	if _, err = m.Apply(ctx, Update{Enabled: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if st := m.Status(); st.Running {
		t.Error("loop still running after disable")
	}
}

func TestManagerValidation(t *testing.T) {
	ctx := context.Background()
	m := NewManager(ctx, memSecrets{}, testEnv(nil), nil, Hooks{})
	for name, u := range map[string]Update{
		"enable without target": {Enabled: ptr(true)},
		"no placeholder":        {Target: ptr("https://127.0.0.1/update")},
		"bad provider":          {Provider: ptr("nope")},
		"quorum over probes":    {ProbeQuorum: ptr(5)},
		"zero interval":         {IntervalSecs: ptr(0)},
	} {
		if _, err := m.Apply(ctx, u); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	if _, err := NewManager(ctx, nil, testEnv(nil), nil, Hooks{}).Apply(ctx, Update{Enabled: ptr(true)}); !errors.Is(err, ErrNoSecrets) {
		t.Errorf("nil secrets err = %v", err)
	}
}

func TestManagerKeepsWriteHistoryAcrossRestart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewManager(ctx, memSecrets{}, testEnv(nil), nil, Hooks{})
	target := "https://127.0.0.1/update?ip={ip}"
	if _, err := m.Apply(ctx, Update{Enabled: ptr(true), Target: ptr(target), IntervalSecs: ptr(120)}); err != nil {
		t.Fatal(err)
	}
	want := Carry{LastIP: "203.0.113.7", LastWrite: time.Now()}
	m.mu.Lock()
	m.up.Seed(want)
	m.mu.Unlock()
	if _, err := m.Apply(ctx, Update{IntervalSecs: ptr(60)}); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	got := m.up.Carry()
	m.mu.Unlock()
	if got != want {
		t.Fatalf("carry after interval change = %+v, want %+v", got, want)
	}
	if _, err := m.Apply(ctx, Update{Target: ptr("https://127.0.0.1/other?ip={ip}")}); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	got = m.up.Carry()
	m.mu.Unlock()
	if got.LastIP != "" {
		t.Fatalf("a new target must write at once, carry = %+v", got)
	}
}

type countProvider struct{ ips []string }

func (c *countProvider) Update(_ context.Context, ip string) error {
	c.ips = append(c.ips, ip)
	return nil
}

func TestSeededUpdaterHonoursMinGapThenUpdatesOnIPChange(t *testing.T) {
	ctx := context.Background()
	cfg := Config{Interval: time.Hour, Quorum: 1, ProbeURLs: []string{"p"}, MinUpdateGap: 5 * time.Minute, Reassert: 24 * time.Hour, FailAlertAfter: 3}
	ip := "203.0.113.7"
	det := &Detector{URLs: cfg.ProbeURLs, Quorum: 1, Fetch: func(context.Context, string) (string, error) { return ip, nil }}
	now := time.Unix(1_000_000, 0)
	prov := &countProvider{}
	first := New(cfg, det, prov, nil, Hooks{})
	first.now = func() time.Time { return now }
	first.Tick(ctx)
	// Restart: a fresh updater seeded from the old one, 54 seconds later.
	now = now.Add(54 * time.Second)
	second := New(cfg, det, prov, nil, Hooks{})
	second.now = func() time.Time { return now }
	second.Seed(first.Carry())
	second.Tick(ctx)
	if len(prov.ips) != 1 {
		t.Fatalf("writes after restart inside gap = %v, want 1", prov.ips)
	}
	ip = "203.0.113.8"
	second.Tick(ctx)
	if len(prov.ips) != 1 {
		t.Fatalf("IP change inside the gap must wait, writes = %v", prov.ips)
	}
	now = now.Add(6 * time.Minute)
	second.Tick(ctx)
	if len(prov.ips) != 2 || prov.ips[1] != ip {
		t.Fatalf("IP change after the gap must update, writes = %v", prov.ips)
	}
}
