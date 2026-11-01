package api

import (
	"context"
	"errors"

	"github.com/GLINCKER/levelrail/internal/heartbeat"
)

// SetJobPings enables per-job heartbeat URLs; without it they are rejected.
func (rt *Router) SetJobPings(p *heartbeat.JobPinger) { rt.jobPings = p }

// persistJobHeartbeat stores or clears a job's ping URLs; the values never reach a response or a log.
func (rt *Router) persistJobHeartbeat(ctx context.Context, taskID string, req scheduledTaskResource) error {
	if req.HeartbeatClear {
		if rt.jobPings != nil {
			return rt.jobPings.Clear(ctx, taskID)
		}
		return nil
	}
	if req.HeartbeatURL == "" && req.HeartbeatFailURL == "" {
		return nil
	}
	if rt.jobPings == nil {
		return errors.New("job heartbeat needs the secrets store (set the master key)")
	}
	if req.HeartbeatURL == "" {
		return errors.New("heartbeat_fail_url needs heartbeat_url")
	}
	return rt.jobPings.Set(ctx, taskID, req.HeartbeatURL, req.HeartbeatFailURL)
}

// withJobHeartbeat adds the host and configured flag, never the URL.
func (rt *Router) withJobHeartbeat(ctx context.Context, res scheduledTaskResource) scheduledTaskResource {
	res.HeartbeatURL, res.HeartbeatFailURL, res.HeartbeatClear = "", "", false
	if rt.jobPings == nil {
		return res
	}
	res.HeartbeatHost, res.HeartbeatConfigured = rt.jobPings.Host(ctx, res.ID)
	return res
}
