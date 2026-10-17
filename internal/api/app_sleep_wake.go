package api

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/appsleep"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Retry-After for the waking response is half the typical wake, kept in a
// range a browser or client will actually honour.
const (
	wakeRetryMinSeconds    = 1
	wakeRetryMaxSeconds    = 10
	wakeRetryNoHistorySecs = 3
	wakingContentTypeHTML  = "text/html; charset=utf-8"
	wakingContentTypeJSON  = "application/json"
	wakingContentTypePlain = "text/plain; charset=utf-8"
	wakingTitle            = "Waking up"
)

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$`)

type wakeTimingStore interface {
	GetWakeTiming(ctx context.Context, name string) (store.WakeTiming, error)
	SetSleepEventReleased(ctx context.Context, id, releasedMs int64) error
}

func (rt *Router) wakeTimingStore() wakeTimingStore {
	s, _ := rt.appSleep.(wakeTimingStore)
	return s
}

func sleepIdleBounds() (lo, hi int) {
	c := appsleep.ConfigFromEnv(nil)
	return c.IdleMin, c.IdleMax
}

type wakeTimingResource struct {
	Samples        int    `json:"samples"`
	ReadyLastMs    *int64 `json:"ready_last_ms,omitempty"`
	ReadyP50Ms     *int64 `json:"ready_p50_ms,omitempty"`
	ReadyP95Ms     *int64 `json:"ready_p95_ms,omitempty"`
	ReleasedLastMs *int64 `json:"released_last_ms,omitempty"`
	ReleasedP50Ms  *int64 `json:"released_p50_ms,omitempty"`
	ReleasedP95Ms  *int64 `json:"released_p95_ms,omitempty"`
}

func toWakeTimingResource(t store.WakeTiming) wakeTimingResource {
	return wakeTimingResource{
		Samples: t.Samples, ReadyLastMs: t.ReadyLastMs, ReadyP50Ms: t.ReadyP50Ms, ReadyP95Ms: t.ReadyP95Ms,
		ReleasedLastMs: t.ReleasedLastMs, ReleasedP50Ms: t.ReleasedP50Ms, ReleasedP95Ms: t.ReleasedP95Ms,
	}
}

type sleepIdleLimits struct {
	MinMinutes     int `json:"min_minutes"`
	MaxMinutes     int `json:"max_minutes"`
	DefaultMinutes int `json:"default_minutes"`
}

func currentIdleLimits() sleepIdleLimits {
	c := appsleep.ConfigFromEnv(nil)
	return sleepIdleLimits{MinMinutes: c.IdleMin, MaxMinutes: c.IdleMax, DefaultMinutes: c.IdleDefault}
}

func (rt *Router) wakeTiming(ctx context.Context, name string) store.WakeTiming {
	s := rt.wakeTimingStore()
	if s == nil {
		return store.WakeTiming{}
	}
	t, err := s.GetWakeTiming(ctx, name)
	if err != nil {
		rt.logger.Warn("api: wake timing failed", slog.String("app", name), slog.String("error", err.Error()))
	}
	return t
}

// markWakeReleased stamps when held requests were let go for event id.
func (rt *Router) markWakeReleased(ctx context.Context, id int64, start time.Time) {
	s := rt.wakeTimingStore()
	if s == nil || id == 0 {
		return
	}
	if err := s.SetSleepEventReleased(ctx, id, time.Since(start).Milliseconds()); err != nil {
		rt.logger.Warn("api: record wake release failed", slog.String("error", err.Error()))
	}
}

func wakeRetryAfterSeconds(t store.WakeTiming) int {
	if t.ReadyP50Ms == nil {
		return wakeRetryNoHistorySecs
	}
	s := int((*t.ReadyP50Ms/2 + 999) / 1000)
	return min(max(s, wakeRetryMinSeconds), wakeRetryMaxSeconds)
}

// typicalWakeSeconds rounds a measured wake up to whole seconds, at least 1.
func typicalWakeSeconds(ms int64) int {
	return max(1, int((ms+999)/1000))
}

func secondsLabel(n int) string {
	if n == 1 {
		return "1 second"
	}
	return strconv.Itoa(n) + " seconds"
}

func (rt *Router) wakingProduct() (name, color string) {
	if rt.brand == nil {
		return "", ""
	}
	if hexColor.MatchString(rt.brand.PrimaryColor) {
		color = rt.brand.PrimaryColor
	}
	return rt.brand.Name, color
}

const wakingPageTemplate = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="refresh" content="%d"><title>%s</title></head>
<body style="font-family:sans-serif;text-align:center;padding:4rem 1rem">
<h1 style="%s">%s</h1>
<p>%s</p>
<p>%s</p>
<p style="color:#6b7280;font-size:.85rem">%s</p>
</body>
</html>
`

// writeWakingResponse answers a request that reached a sleeping app and was
// not released in time. Browsers get a self-refreshing page; every other
// client gets a plain or JSON 503. All carry Retry-After and are never cached.
func (rt *Router) writeWakingResponse(w http.ResponseWriter, r *http.Request, app string, t store.WakeTiming) {
	retry := wakeRetryAfterSeconds(t)
	h := w.Header()
	h.Set("Retry-After", strconv.Itoa(retry))
	h.Set("Cache-Control", "no-store")
	var expect string
	if t.ReadyP50Ms != nil {
		expect = "This app usually wakes in about " + secondsLabel(typicalWakeSeconds(*t.ReadyP50Ms)) + "."
	}
	accept := strings.ToLower(r.Header.Get("Accept"))
	switch {
	case strings.Contains(accept, "text/html"):
		brandName, color := rt.wakingProduct()
		style := ""
		if color != "" {
			style = "color:" + color
		}
		h.Set("Content-Type", wakingContentTypeHTML)
		w.WriteHeader(http.StatusServiceUnavailable)
		page := fmt.Sprintf(wakingPageTemplate, retry, wakingTitle, html.EscapeString(style), wakingTitle,
			"This app was asleep and is starting now. This page will retry on its own.", html.EscapeString(expect), html.EscapeString(brandName))
		_, _ = io.WriteString(w, page) //nolint:gosec // every interpolated value is html-escaped or numeric
	case strings.Contains(accept, "json"):
		h.Set("Content-Type", wakingContentTypeJSON)
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "app is waking up", "app": app, "retry_after_seconds": retry})
	default:
		h.Set("Content-Type", wakingContentTypePlain)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, fmt.Sprintf("%s: the app is waking up. Retry in %ds.\n", app, retry)) //nolint:gosec // text/plain response, not rendered as HTML
	}
}
