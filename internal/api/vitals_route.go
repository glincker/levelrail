package api

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	vitalsMaxRouteLen  = 120
	vitalsMaxSegments  = 6
	vitalsMaxSegmentLn = 24
	vitalsOtherRoute   = "/other"
)

// normalizeVitalsRoute keeps only dictionary-like segments; anything else becomes :id so ids, tokens and emails never persist.
func normalizeVitalsRoute(p string) string {
	if u, err := url.Parse(p); err == nil {
		p = u.Path
	}
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	parts := strings.Split(p, "/")
	segs := make([]string, 0, vitalsMaxSegments)
	for _, s := range parts {
		if s == "" {
			continue
		}
		if len(segs) == vitalsMaxSegments {
			break
		}
		if vitalsPlainSegment(s) {
			segs = append(segs, s)
		} else {
			segs = append(segs, ":id")
		}
	}
	return truncateVitalsRoute("/"+strings.Join(segs, "/"), vitalsMaxRouteLen)
}

// vitalsPlainSegment accepts short lowercase words joined by single hyphens, nothing else.
func vitalsPlainSegment(s string) bool {
	if len(s) > vitalsMaxSegmentLn || strings.HasPrefix(s, "-") || strings.HasSuffix(s, "-") || strings.Contains(s, "--") {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; (c < 'a' || c > 'z') && c != '-' {
			return false
		}
	}
	return true
}

// truncateVitalsRoute cuts at a rune boundary so a multi byte character is never split.
func truncateVitalsRoute(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// vitalsRouteGate caps distinct routes per app per UTC day; extras fold into /other.
type vitalsRouteGate struct {
	mu   sync.Mutex
	apps map[string]*vitalsDayRoutes
}

type vitalsDayRoutes struct {
	day string
	set map[string]struct{}
}

func newVitalsRouteGate() *vitalsRouteGate {
	return &vitalsRouteGate{apps: map[string]*vitalsDayRoutes{}}
}

// admit returns route while the app has room for it today, else /other. The set is seeded from the store once per app per day.
func (g *vitalsRouteGate) admit(ctx context.Context, db vitalsStore, app, route string, limit int, now time.Time) string {
	if route == vitalsOtherRoute {
		return route
	}
	dayStart := now.UTC().Truncate(24 * time.Hour)
	day := dayStart.Format(time.DateOnly)
	g.mu.Lock()
	d := g.apps[app]
	g.mu.Unlock()
	if d == nil || d.day != day {
		seeded := map[string]struct{}{}
		if rs, err := db.VitalsRoutesSince(ctx, app, dayStart, limit+1); err == nil {
			for _, r := range rs {
				seeded[r] = struct{}{}
			}
		}
		g.mu.Lock()
		if cur := g.apps[app]; cur == nil || cur.day != day {
			g.apps[app] = &vitalsDayRoutes{day: day, set: seeded}
		}
		d = g.apps[app]
		g.mu.Unlock()
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := d.set[route]; ok {
		return route
	}
	if len(d.set) >= limit {
		return vitalsOtherRoute
	}
	d.set[route] = struct{}{}
	return route
}
