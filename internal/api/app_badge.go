package api

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// This file implements the opt-in, per-app deploy status badge:
// GET /api/v1/apps/{name}/badge.svg renders a shields.io-style SVG from
// the app's most recent deploy attempt, for embedding in an operator's
// own README. Off by default (store.DesiredService.BadgeEnabled,
// migrations/0278_service_badge_enabled.sql): the route 404s exactly
// like an unknown route until an operator opts a specific app in via
// PUT .../badge, the same enabled-vs-404 shape /public/status already
// establishes (status_page_public.go's servePublicStatus).

// badgeSettingsRequest is PUT /api/v1/apps/{name}/badge's body.
type badgeSettingsRequest struct {
	Enabled bool `json:"enabled"`
}

// badgeSettingsResource is both GET and PUT /api/v1/apps/{name}/badge's
// response.
type badgeSettingsResource struct {
	Enabled bool `json:"enabled"`
}

// handleGetBadgeSettings handles GET /api/v1/apps/{name}/badge: the
// current value of store.DesiredService.BadgeEnabled, so the settings
// toggle has something to read on load.
func (rt *Router) handleGetBadgeSettings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get badge settings failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, badgeSettingsResource{Enabled: svc.BadgeEnabled})
}

// handleSetBadgeSettings handles PUT /api/v1/apps/{name}/badge: opts
// name into (or out of) the public badge.svg route below. AbilityRoot
// (routes.go's registration), the same tier exec-access's own PUT sits
// behind: enabling it hands out an unauthenticated public view of this
// app's deploy status, an explicit, auditable choice rather than a
// default.
func (rt *Router) handleSetBadgeSettings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req badgeSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := rt.apps.SetServiceBadgeEnabled(r.Context(), name, req.Enabled); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		rt.logger.Error("api: set badge settings failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, badgeSettingsResource(req))
}

// handlePublicAppBadge handles GET /api/v1/apps/{name}/badge.svg: the
// one public, unauthenticated route this file adds. It 404s identically
// for an unknown app and a real app with BadgeEnabled false, so a
// prober can never distinguish "no such app" from "app exists, badge
// off" the same way servePublicStatus already treats an unconfigured
// status page.
func (rt *Router) handlePublicAppBadge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		rt.logger.Error("api: public badge: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if !svc.BadgeEnabled {
		http.NotFound(w, r)
		return
	}

	attempts, err := rt.deployAttempts.ListDeployAttempts(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: public badge: list deploy attempts failed", slog.String("error", err.Error()), slog.String("name", name))
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	var latest *store.DeployAttempt
	if len(attempts) > 0 {
		latest = &attempts[0]
	}

	label := "deploy"
	if rt.brand != nil && rt.brand.ShortName != "" {
		label = rt.brand.ShortName + " deploy"
	}
	message, color := badgeStatusMessage(latest)

	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, max-age=60")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(renderBadgeSVG(label, message, color))
}

// Badge pill colors, chosen to read clearly against both GitHub's light
// and dark README rendering.
const (
	badgeColorSuccess = "#2ea043"
	badgeColorFailed  = "#e05d44"
	badgeColorRunning = "#dfb317"
	badgeColorNone    = "#9f9f9f"
)

// badgeStatusMessage turns the most recent deploy attempt (nil meaning
// none yet) into the badge's right-hand text and pill color.
func badgeStatusMessage(a *store.DeployAttempt) (message, color string) {
	if a == nil {
		return "no deploys", badgeColorNone
	}
	switch a.Status {
	case store.DeployAttemptStatusSucceeded:
		return "success, " + humanizeBadgeAge(a.StartedAt) + " ago", badgeColorSuccess
	case store.DeployAttemptStatusFailed:
		return "failed, " + humanizeBadgeAge(a.StartedAt) + " ago", badgeColorFailed
	case store.DeployAttemptStatusRunning, store.DeployAttemptStatusQueued:
		return "deploying", badgeColorRunning
	default:
		return "unknown", badgeColorNone
	}
}

// humanizeBadgeAge renders the time since t in the coarsest unit that
// keeps the badge text short, matching the "2h ago" shape of GitHub's
// own relative timestamps.
func humanizeBadgeAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// badgeCharWidth approximates one character's rendered width at the
// 11px Verdana size shields.io-style badges use; not pixel-exact, but
// close enough that label and message text never overflow their pill.
const badgeCharWidth = 7

// renderBadgeSVG builds a flat, two-pill SVG badge: label on the left
// in neutral gray, message on the right in color. No external image
// library: this is small, static SVG markup assembled with fmt.
func renderBadgeSVG(label, message, color string) []byte {
	labelW := len(label)*badgeCharWidth + 20
	msgW := len(message)*badgeCharWidth + 20
	total := labelW + msgW
	labelCenter := labelW / 2
	msgCenter := labelW + msgW/2

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img" aria-label="%s: %s">`,
		total, badgeXMLEscape(label), badgeXMLEscape(message))
	b.WriteString(`<linearGradient id="s" x2="0" y2="100%"><stop offset="0" stop-color="#bbb" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/></linearGradient>`)
	fmt.Fprintf(&b, `<clipPath id="r"><rect width="%d" height="20" rx="3" fill="#fff"/></clipPath>`, total)
	b.WriteString(`<g clip-path="url(#r)">`)
	fmt.Fprintf(&b, `<rect width="%d" height="20" fill="#555"/>`, labelW)
	fmt.Fprintf(&b, `<rect x="%d" width="%d" height="20" fill="%s"/>`, labelW, msgW, color)
	fmt.Fprintf(&b, `<rect width="%d" height="20" fill="url(#s)"/>`, total)
	b.WriteString(`</g>`)
	b.WriteString(`<g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11">`)
	fmt.Fprintf(&b, `<text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text>`, labelCenter, badgeXMLEscape(label))
	fmt.Fprintf(&b, `<text x="%d" y="14">%s</text>`, labelCenter, badgeXMLEscape(label))
	fmt.Fprintf(&b, `<text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text>`, msgCenter, badgeXMLEscape(message))
	fmt.Fprintf(&b, `<text x="%d" y="14">%s</text>`, msgCenter, badgeXMLEscape(message))
	b.WriteString(`</g></svg>`)
	return []byte(b.String())
}

// badgeXMLEscape escapes label/message text before it lands in SVG
// attribute and element content: both ultimately trace back to
// operator-configured brand/status strings, not request input, but
// escaping costs nothing and removes the question entirely.
func badgeXMLEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
