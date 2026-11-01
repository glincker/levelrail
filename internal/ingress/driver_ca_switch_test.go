package ingress

import (
	"context"
	"testing"
)

func acmeTLSConfig(ca string) *Config {
	return &Config{Apps: Apps{TLS: &TLSApp{Automation: &Automation{Policies: []AutomationPolicy{
		{Subjects: []string{"a.example.org"}, Issuers: []any{NewACMEIssuer("ops@example.org", ca, 0)}},
	}}}}}
}

// Caddy keeps a cached certificate across an ACME CA change, so the driver
// must reset the app set before loading the config for the new CA.
func TestDriverApply_CASwitchResetsCertCache(t *testing.T) {
	orig := loadConfig
	t.Cleanup(func() { loadConfig = orig })
	var payloads [][]byte
	loadConfig = func(b []byte, _ bool) error {
		payloads = append(payloads, append([]byte(nil), b...))
		return nil
	}

	d := New(nil)
	ctx := context.Background()
	staging := "https://acme-staging-v02.api.letsencrypt.org/directory"
	if err := d.Apply(ctx, acmeTLSConfig(staging)); err != nil {
		t.Fatal(err)
	}
	if len(payloads) != 1 {
		t.Fatalf("first apply loads = %d, want 1", len(payloads))
	}
	if err := d.Apply(ctx, acmeTLSConfig("")); err != nil {
		t.Fatal(err)
	}
	if len(payloads) != 3 {
		t.Fatalf("CA switch loads = %d, want reset plus apply", len(payloads))
	}
	if string(payloads[1]) != `{"apps":{}}` && len(payloads[1]) > 40 {
		t.Fatalf("reset payload carries apps: %s", payloads[1])
	}
	if err := d.Apply(ctx, acmeTLSConfig("")); err != nil || len(payloads) != 3 {
		t.Fatalf("same CA must not reload: err=%v loads=%d", err, len(payloads))
	}
}
