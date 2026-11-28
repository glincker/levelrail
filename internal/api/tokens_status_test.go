package api

import (
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

func TestTokenStatusExpiredIsNeverActive(t *testing.T) {
	now := time.Now()
	past, future := now.Add(-22*time.Hour), now.Add(time.Hour)
	tests := []struct {
		name string
		tok  store.APIToken
		want string
	}{
		{"no expiry", store.APIToken{}, tokenStatusActive},
		{"future expiry", store.APIToken{ExpiresAt: &future}, tokenStatusActive},
		{"expired", store.APIToken{ExpiresAt: &past}, tokenStatusExpired},
		{"revoked", store.APIToken{RevokedAt: &past}, tokenStatusRevoked},
		{"revoked and expired is revoked", store.APIToken{RevokedAt: &past, ExpiresAt: &past}, tokenStatusRevoked},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tokenStatus(tc.tok, now); got != tc.want {
				t.Errorf("status = %q, want %q", got, tc.want)
			}
			if got := toTokenResource(tc.tok).Status; got != tc.want {
				t.Errorf("resource status = %q, want %q", got, tc.want)
			}
		})
	}
}
