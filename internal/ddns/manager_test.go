package ddns

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
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
