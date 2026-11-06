package ingress

import (
	"context"
	"strings"
	"sync/atomic"
	"time"
)

// ServiceFacts is what the snapshot needs about one service to name why its
// hosts have no answering replica.
type ServiceFacts struct {
	Name     string
	Domains  []string
	Stopped  bool // suspended by an operator, or zero desired replicas
	Building bool // a deploy attempt is queued or running
	// Containers is every container of the service, running or not, and
	// Running is how many are up. Fresh is true when a running one started
	// recently enough that it may still be passing its readiness check.
	Containers, Running int
	Fresh               bool
}

// FactsSource gathers facts for every service; it is the only DB and Docker
// touch point and runs on the refresh timer, never per request.
type FactsSource func(ctx context.Context) ([]ServiceFacts, error)

// StateOf maps facts to the most specific state: building, stopped, starting,
// unhealthy, else unknown.
func StateOf(f ServiceFacts) HostState {
	switch {
	case f.Building:
		return StateBuilding
	case f.Stopped:
		return StateStopped
	case f.Containers == 0 || (f.Running > 0 && f.Fresh):
		return StateStarting
	case f.Running == 0:
		return StateUnhealthy
	}
	// Up for a while yet unrouted: the route may just be syncing.
	return StateUnknown
}

// DiagnosticSnapshot is a host to state map swapped in whole on each refresh.
type DiagnosticSnapshot struct {
	src   FactsSource
	hosts atomic.Pointer[map[string]HostInfo]
}

// NewDiagnosticSnapshot returns an empty snapshot; every host reads as
// unattached until the first refresh succeeds.
func NewDiagnosticSnapshot(src FactsSource) *DiagnosticSnapshot {
	s := &DiagnosticSnapshot{src: src}
	empty := map[string]HostInfo{}
	s.hosts.Store(&empty)
	return s
}

// Refresh rebuilds the map; on error the previous map stays in place.
func (s *DiagnosticSnapshot) Refresh(ctx context.Context) error {
	facts, err := s.src(ctx)
	if err != nil {
		return err
	}
	m := make(map[string]HostInfo, len(facts))
	for _, f := range facts {
		info := HostInfo{State: StateOf(f), App: f.Name}
		for _, d := range f.Domains {
			if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
				m[d] = info
			}
		}
	}
	s.hosts.Store(&m)
	return nil
}

// Lookup is the HostLookup over the current map. A host no service owns is
// unattached and carries no app name.
func (s *DiagnosticSnapshot) Lookup(_ context.Context, host string) HostInfo {
	if info, ok := (*s.hosts.Load())[host]; ok {
		return info
	}
	return HostInfo{State: StateUnattached}
}

// Run refreshes now and then every interval until ctx ends; interval is read
// each round so a settings change applies live.
func (s *DiagnosticSnapshot) Run(ctx context.Context, interval func(context.Context) time.Duration, onErr func(error)) {
	for {
		if err := s.Refresh(ctx); err != nil && onErr != nil {
			onErr(err)
		}
		d := interval(ctx)
		if d < time.Second {
			d = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(d):
		}
	}
}
