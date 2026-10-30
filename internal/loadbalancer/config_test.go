package loadbalancer

import (
	"strings"
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr string
	}{
		{name: "nil is valid", cfg: nil},
		{name: "empty is valid", cfg: &Config{}},
		{name: "client ip hash is valid", cfg: &Config{Algorithm: AlgoClientIPHash}},
		{name: "cookie name on client ip hash", cfg: &Config{Algorithm: AlgoClientIPHash, CookieName: "x"}, wantErr: "cookie_name"},
		{name: "unknown algorithm", cfg: &Config{Algorithm: "random"}, wantErr: "algorithm"},
		{name: "cookie name on wrong algorithm", cfg: &Config{CookieName: "x"}, wantErr: "cookie_name"},
		{name: "weights on wrong algorithm", cfg: &Config{Weights: []int{1}}, wantErr: "weights only"},
		{name: "weight out of range", cfg: &Config{Algorithm: AlgoWeighted, Weights: []int{0}}, wantErr: "weights[0]"},
		{name: "bad path", cfg: &Config{ActiveHealth: &ActiveHealth{Path: "healthz"}}, wantErr: "must start with /"},
		{name: "timeout not shorter than interval", cfg: &Config{ActiveHealth: &ActiveHealth{Path: "/h", Interval: "2s", Timeout: "2s"}}, wantErr: "shorter than interval"},
		{name: "bad duration", cfg: &Config{DrainTimeout: "soon"}, wantErr: "drain_timeout"},
		{name: "too many retries", cfg: &Config{Retries: &Retries{Count: 11}}, wantErr: "retries.count"},
		{name: "rate limit needs rps", cfg: &Config{RateLimit: &RateLimit{}}, wantErr: "rate_limit.rps"},
		{name: "full valid", cfg: &Config{
			Algorithm: AlgoWeighted, Weights: []int{3, 1},
			ActiveHealth:  &ActiveHealth{Path: "/healthz", Interval: "5s", Timeout: "2s", ExpectStatus: 200},
			PassiveHealth: &PassiveHealth{FailDuration: "30s", MaxFails: 3},
			Retries:       &Retries{Count: 2, TryDuration: "5s"},
			SlowStart:     "30s", DrainTimeout: "15s", RequestTimeout: "30s",
			RateLimit: &RateLimit{RPS: 10, Burst: 20}, UpstreamTLS: &UpstreamTLS{ServerName: "app"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestEffectiveWeightsSlowStart(t *testing.T) {
	now := time.Unix(1000, 0)
	ups := []Upstream{{ID: "a#0", Replica: 0}, {ID: "a#1", Replica: 1}, {ID: "a#2", Replica: 2}}
	cfg := Config{Algorithm: AlgoWeighted, Weights: []int{5, 9}, SlowStart: "10s"}
	seen := map[string]time.Time{
		"a#0": now.Add(-time.Hour),
		"a#1": now.Add(-5 * time.Second),
		"a#2": now,
	}
	got := EffectiveWeights(cfg, ups, seen, now)
	want := []int{5, 5, 1}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("weights = %v, want %v", got, want)
		}
	}
}

func TestConfigDefaults(t *testing.T) {
	tests := []struct {
		name  string
		cfg   Config
		check func(Config) bool
	}{
		{"empty gets round robin", Config{}, func(c Config) bool { return c.Algorithm == AlgoRoundRobin }},
		{"cookie gets default name", Config{Algorithm: AlgoCookie}, func(c Config) bool { return c.CookieName == "lb" }},
		{"cookie keeps custom name", Config{Algorithm: AlgoCookie, CookieName: "sid"}, func(c Config) bool { return c.CookieName == "sid" }},
		{"active health fills probe defaults", Config{ActiveHealth: &ActiveHealth{Path: "/h"}}, func(c Config) bool {
			h := c.ActiveHealth
			return h.Interval == "10s" && h.Timeout == "5s" && h.Passes == 1 && h.Fails == 1
		}},
		{"passive health fills defaults", Config{PassiveHealth: &PassiveHealth{}}, func(c Config) bool {
			return c.PassiveHealth.FailDuration == "30s" && c.PassiveHealth.MaxFails == 1
		}},
		{"retries fill try duration", Config{Retries: &Retries{Count: 2}}, func(c Config) bool { return c.Retries.TryDuration == "5s" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.Defaults(); !tt.check(got) {
				t.Fatalf("Defaults() = %+v", got)
			}
		})
	}
	in := Config{ActiveHealth: &ActiveHealth{Path: "/h"}}
	_ = in.Defaults()
	if in.ActiveHealth.Interval != "" {
		t.Fatal("Defaults mutated the receiver's nested config")
	}
}
