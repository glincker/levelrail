package ingress

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyevents"
)

func init() {
	caddy.RegisterModule(ACMEEventsApp{})
}

// ACMEFailure is the last failed certificate attempt for one hostname.
type ACMEFailure struct {
	Identifier string
	Error      string
	Renewal    bool
	At         time.Time
}

// ACMEFailures remembers each hostname's most recent failed obtain or
// renewal, so the API can show the real CA error instead of "pending".
// A later success clears the entry.
type ACMEFailures struct {
	mu   sync.Mutex
	last map[string]ACMEFailure
}

var defaultACMEFailures = &ACMEFailures{last: map[string]ACMEFailure{}}

// DefaultACMEFailures is the process-wide tracker fed by Caddy's cert events.
func DefaultACMEFailures() *ACMEFailures { return defaultACMEFailures }

// ACMEFailing returns the last error text for identifier; it satisfies
// alerting.ACMEFailureSource.
func (a *ACMEFailures) ACMEFailing(identifier string) (string, bool) {
	f, ok := a.Get(identifier)
	return f.Error, ok
}

// Get returns the last failure for identifier.
func (a *ACMEFailures) Get(identifier string) (ACMEFailure, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	f, ok := a.last[identifier]
	return f, ok
}

// Record stores a failure for identifier.
func (a *ACMEFailures) Record(f ACMEFailure) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.last[f.Identifier] = f
}

// Clear forgets identifier's failure.
func (a *ACMEFailures) Clear(identifier string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.last, identifier)
}

// ClearAll forgets every recorded failure, for when a proxy takes over TLS
// and earlier attempts no longer describe what visitors get.
func (a *ACMEFailures) ClearAll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	clear(a.last)
}

// ACMEEventsApp subscribes the failure tracker to certificate events at
// provision time. Caddy starts apps in random order and the stock events app
// subscribes inside Start, racing the TLS app's async issuance jobs that read
// its subscriber map.
type ACMEEventsApp struct{}

// CaddyModule returns the Caddy module information.
func (ACMEEventsApp) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "acme_events",
		New: func() caddy.Module { return new(ACMEEventsApp) },
	}
}

// Provision binds the tracker before any app starts.
func (ACMEEventsApp) Provision(ctx caddy.Context) error {
	app, err := ctx.App("events")
	if err != nil {
		return fmt.Errorf("ingress: acme events: load events app: %w", err)
	}
	events, ok := app.(*caddyevents.App)
	if !ok {
		return fmt.Errorf("ingress: acme events: events app is %T", app)
	}
	for _, name := range []string{"cert_failed", "cert_obtained"} {
		if err := events.On(name, acmeEventsModule{}); err != nil {
			return fmt.Errorf("ingress: acme events: subscribe %s: %w", name, err)
		}
	}
	return nil
}

// Start implements caddy.App; there is nothing to run.
func (ACMEEventsApp) Start() error { return nil }

// Stop implements caddy.App.
func (ACMEEventsApp) Stop() error { return nil }

type acmeEventsModule struct{}

// Handle records certmagic cert_failed events and clears them on cert_obtained.
func (acmeEventsModule) Handle(_ context.Context, ev caddy.Event) error {
	id, _ := ev.Data["identifier"].(string)
	if id == "" {
		return nil
	}
	switch ev.Name() {
	case "cert_failed":
		renewal, _ := ev.Data["renewal"].(bool)
		defaultACMEFailures.Record(ACMEFailure{
			Identifier: id,
			Error:      fmt.Sprint(ev.Data["error"]),
			Renewal:    renewal,
			At:         ev.Timestamp(),
		})
	case "cert_obtained":
		// A fallback certificate says the public CA failed, not that it recovered.
		if issuer, _ := ev.Data["issuer"].(string); issuer != fallbackKeyInternal {
			defaultACMEFailures.Clear(id)
		}
	}
	return nil
}
