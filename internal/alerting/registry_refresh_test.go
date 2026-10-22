package alerting

import (
	"context"
	"strings"
	"testing"
	"time"
)

type fakeRegistryRefresh []RegistryRefreshHealth

func (f fakeRegistryRefresh) RegistryRefreshHealth(context.Context) ([]RegistryRefreshHealth, error) {
	return f, nil
}

func TestEvaluateRegistryRefreshFailing(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	exp := now.Add(time.Hour)
	tests := []struct {
		name     string
		cred     RegistryRefreshHealth
		wantFire bool
	}{
		{"healthy", RegistryRefreshHealth{Name: "ecr", Preset: "ecr", ExpiresAt: &exp}, false},
		{"failing", RegistryRefreshHealth{Name: "ecr", Preset: "ecr", LastError: "access denied", ExpiresAt: &exp}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, notice, err := EvaluateRegistryRefreshFailing(context.Background(), fakeRegistryRefresh{tt.cred}, Rule{ID: "r1", Kind: KindRegistryRefreshFailing}, now)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if (notice != "") != tt.wantFire {
				t.Fatalf("notice = %q, wantFire %v", notice, tt.wantFire)
			}
			if tt.wantFire && (!strings.Contains(notice, "Refresh now") || !strings.Contains(notice, RegistryRefreshFixPath)) {
				t.Fatalf("notice lacks the fix pointer: %q", notice)
			}
		})
	}
}
