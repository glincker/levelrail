package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/GLINCKER/levelrail/internal/agent/migrationmode"
)

func TestRenderUnitCarriesNoSecretAndIsHardened(t *testing.T) {
	u := renderUnit("/usr/local/bin/x-migrate", "cp.example:9443", "/var/lib/x-migrate")
	for _, want := range []string{"ExecStart=/usr/local/bin/x-migrate run", "NoNewPrivileges=yes", "ProtectSystem=strict", "ReadWritePaths=/var/lib/x-migrate", "CapabilityBoundingSet=CAP_DAC_READ_SEARCH"} {
		if !strings.Contains(u, want) {
			t.Errorf("unit is missing %q", want)
		}
	}
	for _, banned := range []string{"TOKEN", "FINGERPRINT", "mig_"} {
		if strings.Contains(u, banned) {
			t.Errorf("unit must not contain %q", banned)
		}
	}
}

func TestEnded(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"go away", fmt.Errorf("x: %w", migrationmode.ErrGoAway), true},
		{"protocol violation", migrationmode.ErrUnknownControl, true},
		{"revoked", fmt.Errorf("receive: %w", status.Error(codes.PermissionDenied, "revoked")), true},
		{"unauthenticated", status.Error(codes.Unauthenticated, "no"), true},
		{"network drop", status.Error(codes.Unavailable, "down"), false},
		{"plain error", errors.New("boom"), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ended(tc.err); got != tc.want {
				t.Fatalf("ended(%v)=%v want %v", tc.err, got, tc.want)
			}
		})
	}
}
