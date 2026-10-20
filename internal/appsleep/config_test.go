package appsleep

import (
	"testing"
	"time"
)

func TestConfigWakeAndIdleBounds(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want Config
	}{
		{"defaults", nil, Config{IdleMin: DefaultIdleMin, IdleMax: DefaultIdleMax, IdleDefault: DefaultIdleDefault, HoldMin: DefaultHoldMin, HoldMax: DefaultHoldMax, HoldDefault: DefaultHoldDefault, HoldHeadroom: DefaultHoldHeadroom, WakeProbeInterval: DefaultWakeProbeInterval}},
		{"overrides", map[string]string{EnvIdleMin: "10", EnvIdleMax: "120", EnvIdleDefault: "60", EnvHoldMin: "5s", EnvHoldMax: "20s", EnvHoldDefault: "8s", EnvHoldHeadroom: "3", EnvWakeProbeInterval: "100ms"},
			Config{IdleMin: 10, IdleMax: 120, IdleDefault: 60, HoldMin: 5 * time.Second, HoldMax: 20 * time.Second, HoldDefault: 8 * time.Second, HoldHeadroom: 3, WakeProbeInterval: 100 * time.Millisecond}},
		{"inverted idle bounds fall back", map[string]string{EnvIdleMin: "500", EnvIdleMax: "10"},
			Config{IdleMin: DefaultIdleMin, IdleMax: DefaultIdleMax, IdleDefault: DefaultIdleDefault, HoldMin: DefaultHoldMin, HoldMax: DefaultHoldMax, HoldDefault: DefaultHoldDefault, HoldHeadroom: DefaultHoldHeadroom, WakeProbeInterval: DefaultWakeProbeInterval}},
		{"default idle is pulled into range", map[string]string{EnvIdleMin: "60", EnvIdleDefault: "30"},
			Config{IdleMin: 60, IdleMax: DefaultIdleMax, IdleDefault: 60, HoldMin: DefaultHoldMin, HoldMax: DefaultHoldMax, HoldDefault: DefaultHoldDefault, HoldHeadroom: DefaultHoldHeadroom, WakeProbeInterval: DefaultWakeProbeInterval}},
		{"headroom below one is ignored", map[string]string{EnvHoldHeadroom: "0.2"},
			Config{IdleMin: DefaultIdleMin, IdleMax: DefaultIdleMax, IdleDefault: DefaultIdleDefault, HoldMin: DefaultHoldMin, HoldMax: DefaultHoldMax, HoldDefault: DefaultHoldDefault, HoldHeadroom: DefaultHoldHeadroom, WakeProbeInterval: DefaultWakeProbeInterval}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ConfigFromEnv(func(k string) string { return tc.env[k] })
			g := Config{IdleMin: got.IdleMin, IdleMax: got.IdleMax, IdleDefault: got.IdleDefault, HoldMin: got.HoldMin, HoldMax: got.HoldMax, HoldDefault: got.HoldDefault, HoldHeadroom: got.HoldHeadroom, WakeProbeInterval: got.WakeProbeInterval}
			if g != tc.want {
				t.Errorf("config = %+v, want %+v", g, tc.want)
			}
		})
	}
}
