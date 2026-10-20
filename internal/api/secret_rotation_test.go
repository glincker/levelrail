package api

import (
	"testing"
	"time"
)

func TestEffectiveSecretRotationWarnAge_DefaultsWhenUnset(t *testing.T) {
	rt, _ := newTestRouter(t) // no WithSecretRotationWarnAge
	got := rt.effectiveSecretRotationWarnAge()
	if got != defaultSecretRotationWarnAge {
		t.Errorf("effectiveSecretRotationWarnAge() = %v, want the default %v", got, defaultSecretRotationWarnAge)
	}
}

func TestEffectiveSecretRotationWarnAge_UsesConfiguredValue(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithSecretRotationWarnAge(7*24*time.Hour))
	got := rt.effectiveSecretRotationWarnAge()
	if got != 7*24*time.Hour {
		t.Errorf("effectiveSecretRotationWarnAge() = %v, want 7 days", got)
	}
}

func TestSecretIsStale(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithSecretRotationWarnAge(24*time.Hour))

	tests := []struct {
		name      string
		updatedAt time.Time
		want      bool
	}{
		{"zero time is never stale", time.Time{}, false},
		{"just set is not stale", time.Now(), false},
		{"1 hour old is not stale (under 24h threshold)", time.Now().Add(-time.Hour), false},
		{"48 hours old is stale (over 24h threshold)", time.Now().Add(-48 * time.Hour), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rt.secretIsStale(tt.updatedAt); got != tt.want {
				t.Errorf("secretIsStale(%v) = %v, want %v", tt.updatedAt, got, tt.want)
			}
		})
	}
}
