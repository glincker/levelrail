package runtimesettings

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/canarypolicy"
	"github.com/GLINCKER/levelrail/internal/rolloutpolicy"
)

func TestRolloutAndCanaryDefaultsFollowSettings(t *testing.T) {
	t.Cleanup(func() { rolloutpolicy.SetLiveEnv(nil); canarypolicy.SetLiveEnv(nil) })
	svc := New(newFakeStore(), envOf(nil))
	BindRolloutDefaults(svc)

	if got := rolloutpolicy.Resolve(rolloutpolicy.Policy{}, nil, 4); got.MaxSurge != 1 || got.MaxUnavailable != 0 {
		t.Fatalf("defaults = %+v", got)
	}
	for key, val := range map[string]string{KeyRolloutMaxSurge: "50%", KeyCanaryStages: "20,60,100", KeyCanaryInterval: "90s"} {
		raw, _ := json.Marshal(val)
		if _, err := svc.Set(t.Context(), key, raw, "test"); err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
	}
	if got := rolloutpolicy.Resolve(rolloutpolicy.Policy{}, nil, 4).MaxSurge; got != 2 {
		t.Fatalf("live max surge = %d, want 2", got)
	}
	p := canarypolicy.DefaultPolicy(canarypolicy.OSEnv)
	if len(p.Stages) != 3 || p.Stages[0] != 20 || p.Interval != 90*time.Second {
		t.Fatalf("live canary policy = %+v", p)
	}
	if _, err := svc.Set(t.Context(), KeyCanaryStages, json.RawMessage(`"50,10"`), "test"); err == nil {
		t.Fatal("descending stages accepted")
	}
}
