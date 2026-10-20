package store

import (
	"context"
	"testing"
)

func TestIngressSettings_PublicPortRoundTripAndValidation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := IngressSettings{PublicHTTPSPort: 443, TLSTerminatedUpstream: true}
	if err := db.UpdateIngressSettings(ctx, want); err != nil {
		t.Fatalf("UpdateIngressSettings() error = %v", err)
	}
	got, err := db.GetIngressSettings(ctx)
	if err != nil {
		t.Fatalf("GetIngressSettings() error = %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}

	for _, port := range []int{-1, 65536} {
		if err := db.UpdateIngressSettings(ctx, IngressSettings{PublicHTTPSPort: port}); err == nil {
			t.Errorf("port %d: expected validation error", port)
		}
	}
}

func TestIngressSettings_EffectiveFields(t *testing.T) {
	tests := []struct {
		name     string
		s        IngressSettings
		wantACME bool
		wantPort int
	}{
		{"defaults", IngressSettings{}, false, 0},
		{"acme on", IngressSettings{ACMEEnabled: true}, true, 0},
		{"upstream disables acme and defaults to 443", IngressSettings{ACMEEnabled: true, TLSTerminatedUpstream: true}, false, 443},
		{"explicit port wins", IngressSettings{PublicHTTPSPort: 8443, TLSTerminatedUpstream: true}, false, 8443},
		{"port without upstream", IngressSettings{ACMEEnabled: true, PublicHTTPSPort: 443}, true, 443},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.EffectiveACMEEnabled(); got != tt.wantACME {
				t.Errorf("EffectiveACMEEnabled() = %v, want %v", got, tt.wantACME)
			}
			if got := tt.s.PublicLinkPort(); got != tt.wantPort {
				t.Errorf("PublicLinkPort() = %d, want %d", got, tt.wantPort)
			}
		})
	}
}
