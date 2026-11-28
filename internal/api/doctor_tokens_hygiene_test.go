package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCountTokensHygiene(t *testing.T) {
	now := time.Now()
	at := func(d time.Duration) *time.Time { v := now.Add(d); return &v }
	day := 24 * time.Hour
	revoked := now
	recs := []store.APIToken{
		{Name: emergencyTokenName, ExpiresAt: at(30 * day)},
		{Name: emergencyTokenName, ExpiresAt: at(2 * day)},
		{Name: emergencyTokenName, RevokedAt: &revoked},
		{Name: "old", ExpiresAt: at(-100 * day)},
		{Name: "recent", ExpiresAt: at(-5 * day)},
		{Name: "never"},
	}
	got := countTokensHygiene(recs, now, 90, 14*day)
	if got.liveEmergency != 2 || got.expiredPastRetention != 1 || got.expiringSoon != 1 {
		t.Errorf("counts = %+v", got)
	}
	if kept := countTokensHygiene(recs, now, 0, 14*day); kept.expiredPastRetention != 0 {
		t.Errorf("retention 0 keeps tokens forever, got %d", kept.expiredPastRetention)
	}
}
