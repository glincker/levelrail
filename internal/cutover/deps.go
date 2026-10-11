package cutover

import (
	"context"
	"time"
)

// PlanRequest identifies the staged app a plan or run is about.
type PlanRequest struct {
	SessionID string
	SourceID  string
	App       string
	// DNSWrite is whether the caller may change DNS records.
	DNSWrite bool
}

// Planner gathers the facts for one staged app and returns its readiness plan.
type Planner interface {
	Plan(ctx context.Context, req PlanRequest) (Plan, error)
}

// Host starts and stops the staged app here and waits for it.
type Host interface {
	// Route attaches the domains to the app here and starts it.
	Route(ctx context.Context, app string, domains []string) error
	// Unroute detaches the domains and stops the app again.
	Unroute(ctx context.Context, app string, domains []string) error
	// WaitHealthy returns false with a reason when the app is not ready in time.
	WaitHealthy(ctx context.Context, app string, timeout time.Duration) (ok bool, detail string, err error)
}

// ProbeResult is one request through this node's own ingress.
type ProbeResult struct {
	OK     bool
	Status int
	Detail string
}

// Ingress sends a request with the domain's Host header through the real
// ingress listener of this node, whatever DNS currently says.
type Ingress interface {
	Probe(ctx context.Context, domain, path string) (ProbeResult, error)
}

// DNSResult is what an applied record change did, with its undo data.
type DNSResult struct {
	Applied  *Record
	Previous []Record
	Provider string
	Zone     string
	Message  string
}

// DNS changes a domain's record through a connected provider, or restores it.
type DNS interface {
	Apply(ctx context.Context, domain string) (DNSResult, error)
	Restore(ctx context.Context, domain string, previous []Record, applied *Record) error
}

// Proxy writes and removes the managed proxy route of a domain.
type Proxy interface {
	WriteRoute(ctx context.Context, domain string) error
	RemoveRoute(ctx context.Context, domain string) error
}

// Verifier checks a domain through its public name, with the domain doctor.
type Verifier interface {
	Verify(ctx context.Context, app, domain string) (ok bool, detail string, err error)
}

// Deps is everything a Runner acts through. There is deliberately no
// handle on the source platform here.
type Deps struct {
	Planner  Planner
	Host     Host
	Ingress  Ingress
	DNS      DNS
	Proxy    Proxy
	Verifier Verifier
}

// Store persists runs.
type Store interface {
	Create(ctx context.Context, r Run) error
	Get(ctx context.Context, id string) (Run, error)
	Save(ctx context.Context, r Run) error
	List(ctx context.Context, app string, limit int) ([]Run, error)
	ListByState(ctx context.Context, states ...string) ([]Run, error)
}
