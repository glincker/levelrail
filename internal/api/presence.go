package api

import "github.com/glincker/levelrail/internal/telemetry"

// SetPresence lets metrics reads and live streams tell the collectors a viewer is watching. Call before serving.
func (rt *Router) SetPresence(p *telemetry.Presence) { rt.presence = p }

// touchResource marks one resource as viewed so its collector keeps the live cadence while the rest idle.
func (rt *Router) touchResource(id string) { rt.presence.TouchResource(id) }
