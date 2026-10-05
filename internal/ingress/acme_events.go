package ingress

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/caddyserver/caddy/v2"
)

func init() {
	caddy.RegisterModule(acmeEventsModule{})
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

// EventsApp is Caddy's "events" app, used only to observe certificate events.
type EventsApp struct {
	Subscriptions []EventSubscription `json:"subscriptions"`
}

// EventSubscription binds handlers to named events.
type EventSubscription struct {
	Events   []string `json:"events"`
	Handlers []any    `json:"handlers"`
}

func newACMEEventsApp() *EventsApp {
	return &EventsApp{Subscriptions: []EventSubscription{{
		Events:   []string{"cert_failed", "cert_obtained"},
		Handlers: []any{map[string]string{"handler": "acme_tracker"}},
	}}}
}

type acmeEventsModule struct{}

func (acmeEventsModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "events.handlers.acme_tracker",
		New: func() caddy.Module { return new(acmeEventsModule) },
	}
}

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
		defaultACMEFailures.Clear(id)
	}
	return nil
}
