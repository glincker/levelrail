package runtimesettings

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

func TestIngressBodyLimitSetting(t *testing.T) {
	ctx := t.Context()
	svc := New(newFakeStore(), envOf(nil))
	if got := svc.Limit(ctx, KeyIngressMaxBodyBytes); got != 1<<30 {
		t.Fatalf("default = %d", got)
	}
	for _, bad := range []int{1<<20 - 1, 10<<30 + 1} {
		if _, err := svc.Set(ctx, KeyIngressMaxBodyBytes, json.RawMessage(strconv.Itoa(bad)), "t"); err == nil {
			t.Errorf("%d accepted outside the range", bad)
		}
	}
	if !svc.WeakensTo(ctx, KeyIngressMaxBodyBytes, json.RawMessage(strconv.Itoa(2<<30))) {
		t.Error("raising the body limit did not ask for confirmation")
	}
	if svc.WeakensTo(ctx, KeyIngressMaxBodyBytes, json.RawMessage(strconv.Itoa(1<<20))) {
		t.Error("lowering the body limit asked for confirmation")
	}
	if _, err := svc.Set(ctx, KeyIngressMaxBodyBytes, json.RawMessage("4194304"), "t"); err != nil {
		t.Fatal(err)
	}
	if got := svc.Limit(ctx, KeyIngressMaxBodyBytes); got != 4<<20 || svc.Source(ctx, KeyIngressMaxBodyBytes) != SourceSaved {
		t.Fatalf("stored value not read live: %d", got)
	}
}

func TestIngressBodyLimitEnvSeedClamped(t *testing.T) {
	for env, want := range map[string]int{"0": 1 << 30, "100": 1 << 20, "99999999999999": 10 << 30, "8388608": 8 << 20} {
		svc := New(newFakeStore(), envOf(map[string]string{"APP_INGRESS_MAX_BODY_BYTES": env}))
		if got := svc.Limit(t.Context(), KeyIngressMaxBodyBytes); got != want {
			t.Errorf("env %s gives %d, want %d", env, got, want)
		}
	}
}

func TestAccessLogRetentionSetting(t *testing.T) {
	ctx := t.Context()
	svc := New(newFakeStore(), envOf(map[string]string{"APP_ACCESS_LOG_RETENTION": "0"}))
	if got := svc.AccessLogRetention(ctx); got != 8760*time.Hour {
		t.Fatalf("env 0 gives %s", got)
	}
	for _, bad := range []string{`"30m"`, `"9000h"`} {
		if _, err := svc.Set(ctx, KeyAccessLogRetention, json.RawMessage(bad), "t"); err == nil {
			t.Errorf("%s accepted outside the range", bad)
		}
	}
	if _, err := svc.Set(ctx, KeyAccessLogRetention, json.RawMessage(`"2h"`), "t"); err != nil {
		t.Fatal(err)
	}
	if got := svc.AccessLogRetention(ctx); got != 2*time.Hour {
		t.Fatalf("stored value not read live: %s", got)
	}
	short := New(newFakeStore(), envOf(map[string]string{"APP_ACCESS_LOG_RETENTION": "1s"}))
	if got := short.AccessLogRetention(ctx); got != time.Hour {
		t.Fatalf("env 1s gives %s, want the floor", got)
	}
}

func TestFallbackCertsSetting(t *testing.T) {
	ctx := t.Context()
	svc := New(newFakeStore(), envOf(nil))
	if got := svc.String(ctx, KeyIngressFallbackCerts); got != FallbackCertsEager {
		t.Fatalf("default = %q, want eager", got)
	}
	if _, err := svc.Set(ctx, KeyIngressFallbackCerts, json.RawMessage(`"off"`), "t"); err == nil {
		t.Error("unknown mode accepted")
	}
	if _, err := svc.Set(ctx, KeyIngressFallbackCerts, json.RawMessage(`"lazy"`), "t"); err != nil {
		t.Fatal(err)
	}
	if got := svc.String(ctx, KeyIngressFallbackCerts); got != FallbackCertsLazy {
		t.Errorf("after set = %q, want lazy", got)
	}
}

func TestCutoverVerifyReadsSavedThenEnvAndClamps(t *testing.T) {
	svc := New(nil, envOf(map[string]string{
		"APP_CUTOVER_VERIFY_CERT_WARN_DAYS":  "3",
		"APP_CUTOVER_VERIFY_CERT_FAIL_DAYS":  "9",
		"APP_CUTOVER_VERIFY_SLOW_TTFB":       "1ns",
		"APP_CUTOVER_VERIFY_REQUEST_TIMEOUT": "2s",
	}))
	v := svc.CutoverVerify(t.Context())
	if v.CertFailDays != 9 || v.CertWarnDays != 9 {
		t.Fatalf("warn must not fall below fail: %+v", v)
	}
	if v.SlowTTFB != minVerifySlowTTFB || v.Timeout != 2*time.Second || v.HSTSMinAge != 15552000 {
		t.Fatalf("unexpected thresholds: %+v", v)
	}
	d, _ := svc.definition(KeyCutoverVerifyTimeout)
	if d.CheckValue("500ms") == nil || d.CheckValue("5s") != nil {
		t.Fatal("timeout range not enforced")
	}
}
