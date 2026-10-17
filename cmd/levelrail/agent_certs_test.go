package main

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
)

func TestNodeCertThresholds(t *testing.T) {
	tests := []struct {
		name, warn, crit string
		wantW, wantC     time.Duration
	}{
		{"defaults", "", "", alerting.DefaultNodeCertWarning, alerting.DefaultNodeCertCritical},
		{"overrides", "720h", "48h", 720 * time.Hour, 48 * time.Hour},
		{"invalid falls back", "soon", "-1h", alerting.DefaultNodeCertWarning, alerting.DefaultNodeCertCritical},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_NODE_CERT_EXPIRY_WARNING", tt.warn)
			t.Setenv("APP_NODE_CERT_EXPIRY_CRITICAL", tt.crit)
			w, c := nodeCertThresholds()
			if w != tt.wantW || c != tt.wantC {
				t.Errorf("nodeCertThresholds() = %v, %v, want %v, %v", w, c, tt.wantW, tt.wantC)
			}
		})
	}
}
