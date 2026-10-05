package ingress

import (
	"context"
	"errors"
	"testing"

	"github.com/caddyserver/caddy/v2"
)

func TestACMEEventsModule_TracksAndClearsFailures(t *testing.T) {
	var m acmeEventsModule
	ctx := caddy.Context{}
	fail, err := caddy.NewEvent(ctx, "cert_failed", map[string]any{
		"identifier": "1-2-3-4.sslip.io",
		"renewal":    false,
		"error":      errors.New("Timeout during connect (likely firewall problem)"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Handle(context.Background(), fail); err != nil {
		t.Fatal(err)
	}
	got, ok := DefaultACMEFailures().Get("1-2-3-4.sslip.io")
	if !ok || got.Error != "Timeout during connect (likely firewall problem)" || got.Renewal {
		t.Fatalf("failure not recorded: %+v ok=%v", got, ok)
	}

	okEv, err := caddy.NewEvent(ctx, "cert_obtained", map[string]any{"identifier": "1-2-3-4.sslip.io"})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Handle(context.Background(), okEv); err != nil {
		t.Fatal(err)
	}
	if _, ok := DefaultACMEFailures().Get("1-2-3-4.sslip.io"); ok {
		t.Fatal("a successful obtain must clear the recorded failure")
	}
}
