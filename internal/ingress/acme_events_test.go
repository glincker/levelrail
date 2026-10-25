package ingress

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyevents"
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

// Subscribing at provision time (not in the events app's Start) is what keeps
// early certmagic events from racing Caddy's random app start order.
func TestACMEEventsApp_SubscribesAtProvision(t *testing.T) {
	skipUnderLoad(t)
	d := New(testLogger(t))
	cfg := &Config{
		Admin: newAdminConfig("127.0.0.1:" + strconv.Itoa(freePort(t))),
		Apps:  Apps{ACMEEvents: &ACMEEventsApp{}},
	}
	ctx := context.Background()
	if err := d.Apply(ctx, cfg); err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	defer func() {
		if err := d.Stop(ctx); err != nil {
			t.Errorf("Stop() error: %v", err)
		}
	}()

	cctx := caddy.ActiveContext()
	app, err := cctx.App("events")
	if err != nil {
		t.Fatal(err)
	}
	events, ok := app.(*caddyevents.App)
	if !ok {
		t.Fatalf("events app is %T", app)
	}
	const id = "provision-subscribe.example.com"
	if !events.ShouldEmit("cert_failed") || !events.ShouldEmit("cert_obtained") {
		t.Fatal("tracker is not subscribed to cert events after Apply")
	}
	events.Emit(cctx, "cert_failed", map[string]any{"identifier": id, "error": errors.New("boom")})
	if f, ok := DefaultACMEFailures().Get(id); !ok || f.Error != "boom" {
		t.Fatalf("cert_failed not tracked: %+v ok=%v", f, ok)
	}
	events.Emit(cctx, "cert_obtained", map[string]any{"identifier": id})
	if _, ok := DefaultACMEFailures().Get(id); ok {
		t.Fatal("cert_obtained must clear the failure")
	}
}
