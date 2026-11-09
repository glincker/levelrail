package api

import "github.com/GLINCKER/levelrail/internal/telemetry"

// SetPresence lets metrics reads and live streams tell the collectors a viewer is watching. Call before serving.
func (rt *Router) SetPresence(p *telemetry.Presence) { rt.presence = p }
