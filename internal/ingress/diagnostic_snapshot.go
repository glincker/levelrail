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
	// Remote is true when the service is placed on another node, whose
	// containers this process cannot list.
	Remote bool
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
	case f.Remote:
		return StateUnknown
	case f.Containers == 0 || f.Fresh:
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
	// loaded flips on the first successful refresh.
	loaded atomic.Bool
}

// NewDiagnosticSnapshot returns an empty snapshot; every host reads as not
// loaded until the first refresh succeeds.
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
		info := HostInfo{State: StateOf(f)}
		for _, d := range f.Domains {
			if d = NormalizeHost(d); d != "" {
				m[d] = info
			}
		}
	}
	s.hosts.Store(&m)
	s.loaded.Store(true)
	return nil
}

// Lookup is the HostLookup over the current map; a wildcard domain matches
// exactly one label. A host no service owns is unattached.
func (s *DiagnosticSnapshot) Lookup(_ context.Context, host string) HostInfo {
	if !s.loaded.Load() {
		return HostInfo{State: StateNotLoaded}
	}
	host = NormalizeHost(host)
	m := *s.hosts.Load()
	if info, ok := m[host]; ok {
		return info
	}
	if _, rest, ok := strings.Cut(host, "."); ok {
		if info, ok := m["*."+rest]; ok {
			return info
		}
	}
	return HostInfo{State: StateUnattached}
}

// refreshTimeout bounds one Docker and DB pass so a stuck daemon cannot pin the loop.
const refreshTimeout = 10 * time.Second

// Run refreshes now and then every interval until ctx ends; interval is read
// each round so a settings change applies live. While enabled reports false it
// skips the Docker and DB work entirely.
func (s *DiagnosticSnapshot) Run(ctx context.Context, interval func(context.Context) time.Duration, enabled func(context.Context) bool, onErr func(error)) {
	for {
		if enabled == nil || enabled(ctx) {
			rctx, cancel := context.WithTimeout(ctx, refreshTimeout)
			err := s.Refresh(rctx)
			cancel()
			if err != nil && onErr != nil {
				onErr(err)
			}
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
