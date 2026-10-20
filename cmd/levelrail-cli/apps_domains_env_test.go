package main

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestMatchEnvironmentSet(t *testing.T) {
	sets := []apiclient.EnvironmentDomainSet{
		{EnvironmentID: "env_dev", Name: "Development", Kind: "dev"},
		{EnvironmentID: "env_uat", Name: "UAT", Kind: "uat"},
		{EnvironmentID: "env_custom", Name: "dev", Kind: "custom"},
	}
	tests := []struct {
		name, want, wantID string
		ok                 bool
	}{
		{"by id", "env_uat", "env_uat", true},
		{"by name, case-insensitive", "development", "env_dev", true},
		{"name beats kind", "dev", "env_custom", true},
		{"by kind", "uat", "env_uat", true},
		{"missing", "prod", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := matchEnvironmentSet(sets, tt.want)
			if ok != tt.ok || got.EnvironmentID != tt.wantID {
				t.Fatalf("got %q ok=%v, want %q ok=%v", got.EnvironmentID, ok, tt.wantID, tt.ok)
			}
		})
	}
}

func TestAcmeActionText(t *testing.T) {
	for _, action := range []string{"open_port_80", "fix_dns", "wait_rate_limit", "fix_caa", "check_logs", "unknown"} {
		if acmeActionText(action) == "" {
			t.Errorf("empty text for %q", action)
		}
	}
	if acmeActionText("open_port_80") == acmeActionText("fix_dns") {
		t.Error("distinct actions must give distinct steps")
	}
}
