package runtimesettings

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeStore struct {
	rows  map[string]store.RuntimeSettingRow
	lists int
}

func newFakeStore() *fakeStore { return &fakeStore{rows: map[string]store.RuntimeSettingRow{}} }

func (f *fakeStore) ListRuntimeSettings(context.Context) ([]store.RuntimeSettingRow, error) {
	f.lists++
	var out []store.RuntimeSettingRow
	for _, r := range f.rows {
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeStore) SetRuntimeSetting(_ context.Context, key, value, actor string, now time.Time) error {
	f.rows[key] = store.RuntimeSettingRow{Key: key, Value: value, Actor: actor, UpdatedAt: now}
	return nil
}

func (f *fakeStore) DeleteRuntimeSetting(_ context.Context, key string) error {
	delete(f.rows, key)
	return nil
}

func envOf(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func TestResolve_Precedence(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		saved      string
		wantBool   bool
		wantSource string
	}{
		{"default only", nil, "", true, SourceDefault},
		{"env overrides default", map[string]string{"APP_AUDIT_SENSITIVE_READS": "0"}, "", false, SourceEnv},
		{"env false word", map[string]string{"APP_AUDIT_SENSITIVE_READS": "false"}, "", false, SourceEnv},
		{"invalid env falls through", map[string]string{"APP_AUDIT_SENSITIVE_READS": "maybe"}, "", true, SourceDefault},
		{"empty env is unset", map[string]string{"APP_AUDIT_SENSITIVE_READS": ""}, "", true, SourceDefault},
		{"saved beats env", map[string]string{"APP_AUDIT_SENSITIVE_READS": "0"}, "true", true, SourceSaved},
		{"saved false beats default", nil, "false", false, SourceSaved},
		{"invalid saved falls to env", map[string]string{"APP_AUDIT_SENSITIVE_READS": "0"}, "bogus", false, SourceEnv},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := newFakeStore()
			if tc.saved != "" {
				st.rows[KeyAuditSensitiveReads] = store.RuntimeSettingRow{Key: KeyAuditSensitiveReads, Value: tc.saved}
			}
			svc := New(st, envOf(tc.env))
			got, err := svc.Get(t.Context(), KeyAuditSensitiveReads)
			if err != nil {
				t.Fatal(err)
			}
			if got.Source != tc.wantSource || svc.Bool(t.Context(), KeyAuditSensitiveReads) != tc.wantBool {
				t.Fatalf("source=%s bool=%v, want %s %v", got.Source, svc.Bool(t.Context(), KeyAuditSensitiveReads), tc.wantSource, tc.wantBool)
			}
		})
	}
}

func TestSetResetInvalidatesCache(t *testing.T) {
	st := newFakeStore()
	svc := New(st, envOf(nil))
	ctx := t.Context()

	svc.Bool(ctx, KeyWebhookReplayProtection)
	svc.Bool(ctx, KeyWebhookReplayProtection)
	if st.lists != 1 {
		t.Fatalf("hot path hit the store %d times, want 1", st.lists)
	}
	got, err := svc.Set(ctx, KeyWebhookReplayProtection, raw(`false`), "alice")
	if err != nil || got.Source != SourceSaved || string(got.Value) != "false" || got.UpdatedBy != "alice" || got.UpdatedAt == nil {
		t.Fatalf("Set = %+v, %v", got, err)
	}
	if svc.Bool(ctx, KeyWebhookReplayProtection) {
		t.Fatal("saved false not visible to the accessor")
	}
	got, err = svc.Reset(ctx, KeyWebhookReplayProtection)
	if err != nil || got.Source != SourceDefault || string(got.Value) != "true" || got.UpdatedAt != nil {
		t.Fatalf("Reset = %+v, %v", got, err)
	}
}

func TestAccessors(t *testing.T) {
	env := map[string]string{"APP_WEBHOOK_REPLAY_WINDOW": "90s", "APP_BIND_ALLOWED_ROOTS": "/srv/a: /srv/b"}
	svc := New(newFakeStore(), envOf(env))
	ctx := t.Context()
	if d := svc.Duration(ctx, KeyWebhookReplayWindow); d != 90*time.Second {
		t.Fatalf("duration = %v", d)
	}
	if l := svc.List(ctx, KeyBindAllowedRoots); len(l) != 2 || l[0] != "/srv/a" || l[1] != "/srv/b" {
		t.Fatalf("list = %v", l)
	}
	var nilSvc *Service
	if !nilSvc.Bool(ctx, KeyBindBlockCredentialDirs) || nilSvc.Duration(ctx, KeyWebhookReplayWindow) != 10*time.Minute {
		t.Fatal("nil service must serve secure defaults")
	}
	if nilSvc.Bool(ctx, "no.such.key") || svc.Int(ctx, "no.such.key") != 0 {
		t.Fatal("unknown key must be zero")
	}
}

func TestSetValidation(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  string
		ok   bool
	}{
		{"bool ok", KeyAuditSensitiveReads, `true`, true},
		{"bool string ok", KeyAuditSensitiveReads, `"off"`, true},
		{"bool number", KeyAuditSensitiveReads, `1`, false},
		{"bool junk", KeyAuditSensitiveReads, `"perhaps"`, false},
		{"null", KeyAuditSensitiveReads, `null`, false},
		{"duration ok", KeyWebhookReplayWindow, `"2m"`, true},
		{"duration zero", KeyWebhookReplayWindow, `"0s"`, false},
		{"duration negative", KeyWebhookReplayWindow, `"-1m"`, false},
		{"duration junk", KeyWebhookReplayWindow, `"soon"`, false},
		{"duration number", KeyWebhookReplayWindow, `5`, false},
		{"list ok", KeyBindAllowedRoots, `["/srv/a","/data"]`, true},
		{"list empty clears", KeyBindAllowedRoots, `[]`, true},
		{"list colon string", KeyBindAllowedRoots, `"/srv/a:/srv/b"`, true},
		{"list relative", KeyBindAllowedRoots, `["srv/a"]`, false},
		{"list root slash", KeyBindAllowedRoots, `["/"]`, false},
		{"list non string", KeyBindAllowedRoots, `[1]`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := New(newFakeStore(), envOf(nil))
			_, err := svc.Set(t.Context(), tc.key, raw(tc.val), "x")
			var verr *ValidationError
			switch {
			case tc.ok && err != nil:
				t.Fatalf("unexpected error %v", err)
			case !tc.ok && !errors.As(err, &verr):
				t.Fatalf("want ValidationError, got %v", err)
			}
		})
	}
	svc := New(newFakeStore(), envOf(nil))
	if _, err := svc.Set(t.Context(), "nope", raw(`true`), ""); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("unknown key err = %v", err)
	}
}

func TestEnvListInvalidFallsThrough(t *testing.T) {
	svc := New(newFakeStore(), envOf(map[string]string{"APP_BIND_ALLOWED_ROOTS": "/ok:relative"}))
	got, _ := svc.Get(t.Context(), KeyBindAllowedRoots)
	if got.Source != SourceDefault || string(got.Value) != "[]" {
		t.Fatalf("got %+v", got)
	}
}

func TestWeakensTo(t *testing.T) {
	st := newFakeStore()
	st.rows[KeyBindAllowedRoots] = store.RuntimeSettingRow{Key: KeyBindAllowedRoots, Value: `["/srv/a"]`}
	svc := New(st, envOf(nil))
	tests := []struct {
		key, val string
		want     bool
	}{
		{KeyAuditSensitiveReads, `false`, true},
		{KeyAuditSensitiveReads, `true`, false},
		{KeyWebhookReplayProtection, `"0"`, true},
		{KeyWebhookReplayWindow, `"1m"`, true},
		{KeyWebhookReplayWindow, `"20m"`, false},
		{KeyBindAllowedRoots, `[]`, true},
		{KeyBindAllowedRoots, `["/srv/a","/srv/b"]`, true},
		{KeyBindAllowedRoots, `["/srv/a"]`, false},
		{KeyTrustLoopbackForwarded, `false`, false},
		{KeyAuditSensitiveReads, `"junk"`, false},
		{"nope", `false`, false},
	}
	for _, tc := range tests {
		if got := svc.WeakensTo(t.Context(), tc.key, raw(tc.val)); got != tc.want {
			t.Errorf("WeakensTo(%s,%s)=%v want %v", tc.key, tc.val, got, tc.want)
		}
	}
	fresh := New(newFakeStore(), envOf(nil))
	if fresh.WeakensTo(t.Context(), KeyBindAllowedRoots, raw(`["/srv/a"]`)) {
		t.Error("turning an empty allowlist on is stronger, not weaker")
	}
}

func TestIntAcceptsStringAndNumber(t *testing.T) {
	d := Definition{Key: "x.n", Kind: KindInt, Default: "3"}
	for in, want := range map[string]string{`7`: "7", `"8"`: "8"} {
		if got, err := ParseJSON(d, raw(in)); err != nil || got != want {
			t.Errorf("ParseJSON(%s) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{`1.5`, `"x"`, `true`} {
		if _, err := ParseJSON(d, raw(bad)); err == nil {
			t.Errorf("ParseJSON(%s) accepted", bad)
		}
	}
}

func TestPendingRestartAndBootValue(t *testing.T) {
	st := newFakeStore()
	svc := New(st, envOf(nil))
	ctx := t.Context()
	if err := svc.CaptureBoot(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(ctx, KeyTrustLoopbackForwarded)
	if got.PendingRestart || got.Apply != string(ApplyRestart) {
		t.Fatalf("fresh boot pending = %+v", got)
	}
	got, err := svc.Set(ctx, KeyTrustLoopbackForwarded, raw(`false`), "a")
	if err != nil || !got.PendingRestart {
		t.Fatalf("after change = %+v, %v", got, err)
	}
	if !svc.Bool(ctx, KeyTrustLoopbackForwarded) {
		t.Fatal("restart-kind accessor must keep the boot value")
	}
	if pending, _ := svc.AnyPendingRestart(ctx); !pending {
		t.Fatal("AnyPendingRestart = false")
	}
	got, _ = svc.Set(ctx, KeyTrustLoopbackForwarded, raw(`true`), "a")
	if got.PendingRestart {
		t.Fatal("setting back to the boot value must clear pending")
	}
	live, _ := svc.Set(ctx, KeyAuditSensitiveReads, raw(`false`), "a")
	if live.PendingRestart {
		t.Fatal("live setting reported pending_restart")
	}
}

func TestOnboardingReview(t *testing.T) {
	svc := New(newFakeStore(), envOf(nil))
	ctx := t.Context()
	items, err := svc.Onboarding(ctx)
	if err != nil || len(items) != 4 {
		t.Fatalf("onboarding items = %d, %v", len(items), err)
	}
	if ok, _ := svc.Reviewed(ctx); ok {
		t.Fatal("reviewed before ack")
	}
	if err := svc.AckReviewed(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := svc.Reviewed(ctx); !ok {
		t.Fatal("not reviewed after ack")
	}
	all, _ := svc.All(ctx)
	for _, s := range all {
		if s.Key == onboardingKey {
			t.Fatal("onboarding.reviewed must not be a listed setting")
		}
	}
}

func TestRegistryDefaultsAreValidAndSecure(t *testing.T) {
	want := map[string]string{
		KeyWebhookReplayProtection: "true", KeyAuditSensitiveReads: "true", KeyBindBlockCredentialDirs: "true",
	}
	svc := New(nil, envOf(nil))
	for _, d := range Definitions() {
		if _, err := canonicalFromString(d, d.Default); err != nil {
			t.Errorf("%s default invalid: %v", d.Key, err)
		}
		if v, ok := want[d.Key]; ok && svc.value(t.Context(), d.Key) != v {
			t.Errorf("%s not secure by default", d.Key)
		}
	}
	if len(Definitions()) != 6 {
		t.Fatalf("registry size = %d", len(Definitions()))
	}
}
