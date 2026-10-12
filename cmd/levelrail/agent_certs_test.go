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

func TestAgentRequireCSR(t *testing.T) {
	tests := []struct {
		name, value string
		want        bool
	}{
		{"unset requires a CSR", "", true},
		{"false allows legacy agents", "false", false},
		{"zero allows legacy agents", "0", false},
		{"true requires", "true", true},
		{"garbage fails closed", "nope", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_AGENT_REQUIRE_CSR", tt.value)
			if got := agentRequireCSR(); got != tt.want {
				t.Errorf("agentRequireCSR() = %v, want %v", got, tt.want)
			}
		})
	}
}
