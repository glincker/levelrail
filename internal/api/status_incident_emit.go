package api

import "context"

// EmitStatusIncident lets the monitor linker queue incident_opened and incident_resolved
// for incidents it writes outside the HTTP handlers.
func (rt *Router) EmitStatusIncident(ctx context.Context, id string, opened bool) {
	rt.emitStatusIncidentEvent(ctx, id, opened)
}
