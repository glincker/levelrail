package ingress

import (
	"bytes"
	"context"
	"errors"
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

func stubLoads(t *testing.T, fail func(n int, b []byte) error) *[][]byte {
	t.Helper()
	origLoad, origVal := loadConfig, validateConfig
	t.Cleanup(func() { loadConfig, validateConfig = origLoad, origVal })
	validateConfig = func([]byte) error { return nil }
	var got [][]byte
	loadConfig = func(b []byte, _ bool) error {
		got = append(got, append([]byte(nil), b...))
		if fail != nil {
			return fail(len(got), b)
		}
		return nil
	}
	return &got
}

const stagingCA = "https://acme-staging-v02.api.letsencrypt.org/directory"

func TestDriverApply_CASwitchSecondLoadFailureRestores(t *testing.T) {
	loads := stubLoads(t, func(n int, _ []byte) error {
		if n == 3 {
			return errors.New("boom")
		}
		return nil
	})
	d := New(nil)
	ctx := context.Background()
	if err := d.Apply(ctx, acmeTLSConfig(stagingCA)); err != nil {
		t.Fatal(err)
	}
	first := append([]byte(nil), d.lastApplied...)
	if err := d.Apply(ctx, acmeTLSConfig("")); err == nil {
		t.Fatal("want error")
	}
	if len(*loads) != 4 || !bytes.Equal((*loads)[3], first) {
		t.Fatalf("previous config not restored: %d loads", len(*loads))
	}
	if !bytes.Equal(d.lastApplied, first) || d.lastCASig != "acme|"+stagingCA {
		t.Fatal("state must stay on the previous config and CA")
	}
	if err := d.Apply(ctx, acmeTLSConfig("")); err == nil || len(*loads) != 4 {
		t.Fatalf("same failing payload must not retry: err=%v loads=%d", err, len(*loads))
	}
}

func TestDriverApply_CASwitchValidationFailureSkipsReset(t *testing.T) {
	loads := stubLoads(t, nil)
	d := New(nil)
	ctx := context.Background()
	if err := d.Apply(ctx, acmeTLSConfig(stagingCA)); err != nil {
		t.Fatal(err)
	}
	validateConfig = func([]byte) error { return errors.New("bad") }
	if err := d.Apply(ctx, acmeTLSConfig("")); err == nil {
		t.Fatal("want error")
	}
	if len(*loads) != 1 || d.lastCASig != "acme|"+stagingCA {
		t.Fatalf("validation failure must not reset: loads=%d", len(*loads))
	}
}

func TestACMECASig_SameCADoesNotReset(t *testing.T) {
	loads := stubLoads(t, nil)
	d := New(nil)
	ctx := context.Background()
	if err := d.Apply(ctx, acmeTLSConfig("")); err != nil {
		t.Fatal(err)
	}
	two := acmeTLSConfig("")
	two.Apps.TLS.Automation.Policies = append(two.Apps.TLS.Automation.Policies,
		AutomationPolicy{Subjects: []string{"b.example.org"}, Issuers: []any{NewACMEIssuer("ops@example.org", acmeProductionURL, 0)}})
	if err := d.Apply(ctx, two); err != nil {
		t.Fatal(err)
	}
	if len(*loads) != 2 {
		t.Fatalf("same CA must load once per apply, got %d", len(*loads))
	}
	if acmeCASignature(acmeTLSConfig("")) != acmeCASignature(acmeTLSConfig(acmeProductionURL)) {
		t.Fatal("default CA must equal production URL")
	}
	if acmeCASignature(acmeTLSConfig("")) == acmeCASignature(acmeTLSConfig(stagingCA)) {
		t.Fatal("staging must differ")
	}
}

func TestDriverApply_CASigKeptAcrossACMEDisabled(t *testing.T) {
	loads := stubLoads(t, nil)
	d := New(nil)
	ctx := context.Background()
	for _, c := range []*Config{acmeTLSConfig(stagingCA), {}, acmeTLSConfig("")} {
		if err := d.Apply(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	if len(*loads) != 4 {
		t.Fatalf("re-enabling with another CA must reset, loads=%d", len(*loads))
	}
}

func TestDriverApply_CASwitchRetriesAfterBackoff(t *testing.T) {
	t.Setenv("APP_INGRESS_CA_RETRY_BACKOFF", "0s")
	loads := stubLoads(t, func(n int, _ []byte) error {
		if n == 3 {
			return errors.New("transient")
		}
		return nil
	})
	d := New(nil)
	ctx := context.Background()
	_ = d.Apply(ctx, acmeTLSConfig(stagingCA))
	if err := d.Apply(ctx, acmeTLSConfig("")); err == nil {
		t.Fatal("want error")
	}
	if err := d.Apply(ctx, acmeTLSConfig("")); err != nil || len(*loads) < 6 {
		t.Fatalf("identical payload must retry after backoff: err=%v loads=%d", err, len(*loads))
	}
}
