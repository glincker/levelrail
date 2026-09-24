package api

import (
	"context"
	"errors"
	"net/http"
	"time"
)

const readyzCheckTimeout = 2 * time.Second

// ReadinessProbes are the cheap checks GET /readyz runs. A nil probe is skipped.
type ReadinessProbes struct {
	Database      func(ctx context.Context) error
	Migrations    func(ctx context.Context) error
	EngineStarted func() bool
}

// WithReadinessProbes enables the database, migration and engine checks on GET /readyz.
func WithReadinessProbes(p ReadinessProbes) Option {
	return func(rt *Router) { rt.readiness = p }
}

type readyzCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Fatal  bool   `json:"fatal"`
	Detail string `json:"detail,omitempty"`
}

type readyzResponse struct {
	Ready  bool          `json:"ready"`
	Checks []readyzCheck `json:"checks"`
}

// handleReadyz handles GET /readyz: unauthenticated, cheap readiness.
// Docker being down is reported as degraded but does not fail readiness,
// so the dashboard stays reachable to show the outage. Error detail is
// generic to keep internals out of an unauthenticated response.
func (rt *Router) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readyzCheckTimeout)
	defer cancel()

	resp := readyzResponse{Ready: true, Checks: []readyzCheck{}}
	add := func(name string, fatal bool, err error, failDetail string) {
		c := readyzCheck{Name: name, Status: "ok", Fatal: fatal}
		if err != nil {
			c.Detail = failDetail
			if fatal {
				c.Status = "failing"
				resp.Ready = false
			} else {
				c.Status = "degraded"
			}
		}
		resp.Checks = append(resp.Checks, c)
	}

	if rt.readiness.Database != nil {
		add("database", true, rt.readiness.Database(ctx), "database unreachable")
	}
	if rt.readiness.Migrations != nil {
		add("migrations", true, rt.readiness.Migrations(ctx), "migrations not applied")
	}
	if rt.readiness.EngineStarted != nil {
		var err error
		if !rt.readiness.EngineStarted() {
			err = errors.New("not started")
		}
		add("reconcile_engine", true, err, "reconcile engine not started")
	}
	if rt.dockerPinger != nil {
		add("docker", false, rt.dockerPinger.Ping(ctx), "docker daemon unreachable")
	}

	status := http.StatusOK
	if !resp.Ready {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, resp)
}
