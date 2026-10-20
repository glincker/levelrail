package ingress

import (
	"testing"
	"time"
)

func i64(v int64) *int64 { return &v }

func TestWithOverrides(t *testing.T) {
	none := func(string) string { return "" }
	tests := []struct {
		name   string
		o      EdgeOverrides
		env    map[string]string
		check  func(Hardening) bool
		errMsg bool
	}{
		{"nothing set keeps defaults", EdgeOverrides{}, nil, func(h Hardening) bool {
			return h.MaxBodyBytes == DefaultHardening().MaxBodyBytes && len(h.TrustedProxies) == 0
		}, false},
		{"setting applies", EdgeOverrides{TrustedProxies: "10.0.0.1", MaxBodyBytes: i64(5), MaxHeaderBytes: i64(4096), ReadHeaderTimeoutSecs: i64(3), IdleTimeoutSecs: i64(7)}, nil,
			func(h Hardening) bool {
				return h.MaxBodyBytes == 5 && h.MaxHeaderBytes == 4096 && h.ReadHeaderTimeout == 3*time.Second && h.IdleTimeout == 7*time.Second && len(h.TrustedProxies) == 1
			}, false},
		{"env wins over setting", EdgeOverrides{TrustedProxies: "10.0.0.1", MaxBodyBytes: i64(5)}, map[string]string{EnvTrustedProxies: "192.0.2.0/24", EnvMaxBodyBytes: "9"},
			func(h Hardening) bool { return h.MaxBodyBytes == 1<<30 && len(h.TrustedProxies) == 0 }, false},
		{"private_ranges shorthand", EdgeOverrides{TrustedProxies: "private_ranges"}, nil, func(h Hardening) bool { return len(h.TrustedProxies) == len(privateRanges) }, false},
		{"bad proxy errors", EdgeOverrides{TrustedProxies: "nope"}, nil, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			getenv := none
			if tc.env != nil {
				getenv = func(k string) string { return tc.env[k] }
			}
			h, err := DefaultHardening().WithOverrides(tc.o, getenv)
			if (err != nil) != tc.errMsg {
				t.Fatalf("err = %v", err)
			}
			if tc.check != nil && !tc.check(h) {
				t.Fatalf("unexpected hardening %+v", h)
			}
		})
	}
}

func TestBodyLimitFor(t *testing.T) {
	h := &Hardening{MaxBodyBytes: 100}
	tests := []struct {
		name     string
		h        *Hardening
		override int64
		want     int64
	}{
		{"instance cap", h, 0, 100},
		{"app override beats instance", h, 7, 7},
		{"nil hardening with override", nil, 9, 9},
		{"nil hardening no override", nil, 0, 0},
		{"unlimited instance, app cap", &Hardening{}, 5, 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.h.bodyLimitFor(tc.override)
			if tc.want == 0 {
				if got != nil {
					t.Fatalf("want nil, got %v", got)
				}
				return
			}
			rb, ok := got.(RequestBodyHandler)
			if !ok || rb.MaxSize != tc.want {
				t.Fatalf("got %v, want max %d", got, tc.want)
			}
		})
	}
}
