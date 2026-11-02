package ingress

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
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
	loads := stubLoads(t, func(n int, _ []byte) error {
		if n == 3 {
			return errors.New("transient")
		}
		return nil
	})
	d := New(nil)
	d.SetCARetryBackoff(func(context.Context) (time.Duration, time.Duration) { return time.Nanosecond, time.Nanosecond })
	ctx := context.Background()
	_ = d.Apply(ctx, acmeTLSConfig(stagingCA))
	if err := d.Apply(ctx, acmeTLSConfig("")); err == nil {
		t.Fatal("want error")
	}
	time.Sleep(time.Millisecond)
	if err := d.Apply(ctx, acmeTLSConfig("")); err != nil || len(*loads) < 6 {
		t.Fatalf("identical payload must retry after backoff: err=%v loads=%d", err, len(*loads))
	}
}

func TestDriverApply_CASwitchBackoffGrowsAndExplains(t *testing.T) {
	stubLoads(t, func(n int, _ []byte) error {
		if n == 3 {
			return errors.New("port busy")
		}
		return nil
	})
	d := New(nil)
	d.SetCARetryBackoff(func(context.Context) (time.Duration, time.Duration) { return time.Minute, 3 * time.Minute })
	for fails, want := range []time.Duration{time.Minute, 2 * time.Minute, 3 * time.Minute, 3 * time.Minute} {
		if got := d.backoffFor(context.Background(), fails+1); got != want {
			t.Fatalf("fails=%d delay=%s want %s", fails+1, got, want)
		}
	}
	ctx := context.Background()
	_ = d.Apply(ctx, acmeTLSConfig(stagingCA))
	_ = d.Apply(ctx, acmeTLSConfig(""))
	err := d.Apply(ctx, acmeTLSConfig(""))
	if err == nil || !strings.Contains(err.Error(), "previous CA switch failed, retrying after") {
		t.Fatalf("backoff error = %v", err)
	}
	if !strings.Contains(CASwitchNotice(), "retrying after") {
		t.Fatalf("notice = %q", CASwitchNotice())
	}
}

func TestDriverApply_NoResetWithoutKnownGoodConfig(t *testing.T) {
	loads := stubLoads(t, func(n int, _ []byte) error {
		if n == 2 {
			return errors.New("bad")
		}
		return nil
	})
	d := New(nil)
	ctx := context.Background()
	_ = d.Apply(ctx, acmeTLSConfig(stagingCA)) // load 1: ok
	// A failed normal load clears lastApplied but must keep lastGood.
	bad := acmeTLSConfig(stagingCA)
	bad.Apps.TLS.Automation.Policies[0].Subjects = []string{"x.example.org"}
	if err := d.Apply(ctx, bad); err == nil {
		t.Fatal("want error")
	}
	if d.lastGood == nil {
		t.Fatal("lastGood must survive a failed normal load")
	}
	d.mu.Lock()
	d.lastGood = nil
	d.mu.Unlock()
	before := len(*loads)
	if err := d.Apply(ctx, acmeTLSConfig("")); err != nil {
		t.Fatal(err)
	}
	if len(*loads)-before != 1 {
		t.Fatalf("no known-good config: want a single load without reset, got %d", len(*loads)-before)
	}
}

func TestDriverApply_RestoreFailureRetriesNextTick(t *testing.T) {
	stubLoads(t, func(n int, _ []byte) error {
		if n >= 3 {
			return errors.New("down")
		}
		return nil
	})
	d := New(nil)
	ctx := context.Background()
	_ = d.Apply(ctx, acmeTLSConfig(stagingCA))
	_ = d.Apply(ctx, acmeTLSConfig(""))
	if d.failedReset != nil {
		t.Fatal("failed restore must clear failedReset so the next tick retries")
	}
}
