package runtimesettings

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/abuse"
)

func TestAbuseSettingsMatchLimitsDefaults(t *testing.T) {
	svc := New(newFakeStore(), envOf(nil))
	if got, want := svc.AbuseLimits(t.Context()), abuse.DefaultLimits(); got != want {
		t.Fatalf("settings defaults %+v differ from limits defaults %+v", got, want)
	}
}

func TestAbuseSettingsEnvSeedsAndRangeChecks(t *testing.T) {
	svc := New(newFakeStore(), envOf(map[string]string{abuse.EnvMaxRPS: "500", abuse.EnvDryRunMax: "2h"}))
	l := svc.AbuseLimits(t.Context())
	if l.MaxRPS != 500 || l.DryRunMax.Hours() != 2 {
		t.Fatalf("env seeds not applied: %+v", l)
	}
	bad := map[string]string{
		KeyAbuseMaxRPS: `"0"`, KeyAbuseMaxBurst: `0`, KeyAbuseShards: `5000`, KeyAbuseIdleEvict: `"1s"`,
		KeyAbuseSuggestShare: `"2"`, KeyAbuseDryRunTop: `101`, KeyAbuseMaxTrackedClients: `10`,
	}
	for key, v := range bad {
		if _, err := svc.ValidateForWrite(t.Context(), key, raw(v), WriteOptions{}); err == nil {
			t.Errorf("%s accepted out of range value %s", key, v)
		}
	}
}

func TestAbuseRPSChangeAppliesWithoutRestart(t *testing.T) {
	svc := New(newFakeStore(), envOf(nil))
	ctx := t.Context()
	if _, err := svc.Set(ctx, KeyAbuseMaxRPS, raw(`"250"`), "test"); err != nil {
		t.Fatal(err)
	}
	if got := svc.AbuseLimits(ctx).MaxRPS; got != 250 {
		t.Fatalf("max rps = %v after save, want 250", got)
	}
	p := abuse.Policy{Mode: abuse.ModeEnforce, RPS: 300, Burst: 10}
	if p.Validate(svc.AbuseLimits(ctx)) == nil {
		t.Fatal("policy above the saved ceiling validated")
	}
	if _, err := svc.Set(ctx, KeyAbuseMaxRPS, raw(`"400"`), "test"); err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(svc.AbuseLimits(ctx)); err != nil {
		t.Fatalf("raised ceiling not applied live: %v", err)
	}
}

func TestAbuseFloatSettingsRejectNaNAndInf(t *testing.T) {
	def := abuse.DefaultLimits()
	for _, seed := range []string{"NaN", "nan", "Inf", "+Inf", "-Inf", "1e999"} {
		svc := New(newFakeStore(), envOf(map[string]string{abuse.EnvMaxRPS: seed, abuse.EnvDefaultRPS: seed}))
		l := svc.AbuseLimits(t.Context())
		if l.MaxRPS != def.MaxRPS || l.DefaultRPS != def.DefaultRPS {
			t.Errorf("env seed %q: max %v default %v, want the defaults", seed, l.MaxRPS, l.DefaultRPS)
		}
		for _, key := range []string{KeyAbuseMaxRPS, KeyAbuseDefaultRPS} {
			if _, err := svc.ValidateForWrite(t.Context(), key, raw(`"`+seed+`"`), WriteOptions{}); err == nil {
				t.Errorf("%s accepted %q", key, seed)
			}
		}
	}
}
