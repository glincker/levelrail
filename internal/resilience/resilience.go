// Package resilience judges whether the data plane would keep serving if the control plane went away.
package resilience

import (
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
)

// Check statuses.
const (
	StatusPass    = "pass"
	StatusFail    = "fail"
	StatusWarn    = "warn"
	StatusUnknown = "unknown"
)

// FixSaveSnapshot is the fix that rewrites the route snapshot from the applied config.
const FixSaveSnapshot = "save_snapshot"

// Check IDs.
const (
	CheckRouteSnapshot = "route_snapshot"
	CheckCertCache     = "cert_cache"
	CheckAgentLink     = "agent_link"
	CheckContainers    = "container_restart"
	CheckTelemetry     = "telemetry_buffer"
)

const staleNodeAfter = 2 * time.Minute

// Check is one survival property of the ingress or a node.
type Check struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// Scope groups the checks of the ingress or one node.
type Scope struct {
	Kind   string  `json:"kind"`
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Checks []Check `json:"checks"`
}

// Report is the whole resilience check.
type Report struct {
	GeneratedAt time.Time `json:"generated_at"`
	Simulated   bool      `json:"simulated"`
	Overall     string    `json:"overall"`
	Scopes      []Scope   `json:"scopes"`
}

// IngressInput is what the control plane knows about the persisted ingress state.
type IngressInput struct {
	SnapshotEnabled bool
	SavedAt         time.Time
	SaveError       string
	// Verified is set when the snapshot was opened as a restart would; VerifyErr is the outcome.
	Verified    bool
	VerifyErr   error
	ConfigBytes int
	CertKeys    int
	CertErr     error
}

// NodeInput is one node as the control plane last saw it.
type NodeInput struct {
	ID       string
	Name     string
	Local    bool
	Status   string
	LastSeen *time.Time
}

// Evaluate builds the report. simulate adds the restart dry run to the ingress checks.
func Evaluate(now time.Time, simulate bool, in IngressInput, nodes []NodeInput) Report {
	r := Report{GeneratedAt: now, Simulated: simulate}
	r.Scopes = append(r.Scopes, Scope{Kind: "ingress", ID: "ingress", Name: "Ingress", Checks: []Check{
		snapshotCheck(now, simulate, in), certCheck(in),
	}})
	for _, n := range nodes {
		r.Scopes = append(r.Scopes, Scope{Kind: "node", ID: n.ID, Name: n.Name, Checks: nodeChecks(now, n)})
	}
	r.Overall = overall(r.Scopes)
	return r
}

func snapshotCheck(now time.Time, simulate bool, in IngressInput) Check {
	c := Check{ID: CheckRouteSnapshot}
	if !in.SnapshotEnabled {
		c.Status = StatusFail
		c.Detail = "Route snapshot is off (no master key loaded), so a restart serves nothing until the first reconcile."
		return c
	}
	if in.SaveError != "" {
		c.Status, c.Fix = StatusFail, FixSaveSnapshot
		c.Detail = "The last snapshot write failed: " + in.SaveError
		return c
	}
	if in.SavedAt.IsZero() {
		c.Status, c.Fix = StatusFail, FixSaveSnapshot
		c.Detail = "No route snapshot has been written yet."
		return c
	}
	age := now.Sub(in.SavedAt).Round(time.Second)
	if simulate && in.Verified {
		if errors.Is(in.VerifyErr, ingress.ErrSnapshotStale) {
			c.Status, c.Fix = StatusFail, FixSaveSnapshot
			c.Detail = "The snapshot was taken with different listeners and would not be replayed on restart."
			return c
		}
		if in.VerifyErr != nil {
			c.Status, c.Fix = StatusFail, FixSaveSnapshot
			c.Detail = "The snapshot cannot be opened and a restart would ignore it: " + in.VerifyErr.Error()
			return c
		}
		c.Status = StatusPass
		c.Detail = fmt.Sprintf("Dry run: a restart would serve %d bytes of routes from the snapshot saved %s ago, no database needed.", in.ConfigBytes, age)
		return c
	}
	c.Status = StatusPass
	c.Detail = fmt.Sprintf("Snapshot saved %s ago. Run the outage simulation to prove a restart can use it.", age)
	return c
}

func certCheck(in IngressInput) Check {
	c := Check{ID: CheckCertCache}
	switch {
	case in.CertErr != nil:
		c.Status = StatusFail
		c.Detail = "Certificate storage cannot be read: " + in.CertErr.Error()
	case in.CertKeys == 0:
		c.Status = StatusUnknown
		c.Detail = "No certificates are stored yet (no domain has been issued one)."
	default:
		c.Status = StatusPass
		c.Detail = fmt.Sprintf("%d stored certificate entries live in the local database, so a restart reuses them without contacting the CA.", in.CertKeys)
	}
	return c
}

func nodeChecks(now time.Time, n NodeInput) []Check {
	link := Check{ID: CheckAgentLink}
	switch {
	case n.Local:
		link.Status, link.Detail = StatusPass, "Runs inside the control plane process."
	case n.LastSeen == nil:
		link.Status, link.Detail = StatusUnknown, "The agent has never reported in."
	case n.Status != "online" || now.Sub(*n.LastSeen) > staleNodeAfter:
		link.Status = StatusWarn
		link.Detail = fmt.Sprintf("The agent was last seen %s ago. Its containers keep running; it reconnects on its own.", now.Sub(*n.LastSeen).Round(time.Second))
	default:
		link.Status, link.Detail = StatusPass, "The agent is connected and reports its containers."
	}
	return []Check{
		link,
		{ID: CheckContainers, Status: StatusWarn, Detail: "Running containers keep serving without the control plane. The container restart policy is off by design (Docker and Podman alike), so after a host reboot during an outage they stay down until the control plane returns."},
		{ID: CheckTelemetry, Status: StatusPass, Detail: "Metrics and logs are stored on the node and pulled on demand, so nothing is lost or buffered while disconnected."},
	}
}

func overall(scopes []Scope) string {
	worst := StatusPass
	rank := map[string]int{StatusPass: 0, StatusUnknown: 1, StatusWarn: 2, StatusFail: 3}
	for _, s := range scopes {
		for _, c := range s.Checks {
			if rank[c.Status] > rank[worst] {
				worst = c.Status
			}
		}
	}
	return worst
}
