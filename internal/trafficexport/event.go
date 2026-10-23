package trafficexport

import (
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
)

// Event is the vendor-neutral record of one finished edge request. It never
// carries a query string, cookie, auth header or body. ClientIP, UserAgent and
// VisitorID are set only for sinks that opt in (see Policy).
type Event struct {
	Time          time.Time `json:"time"`
	App           string    `json:"app"`
	Host          string    `json:"host"`
	Method        string    `json:"method"`
	Path          string    `json:"path"`
	Status        int       `json:"status"`
	DurationMS    float64   `json:"duration_ms"`
	BytesIn       int64     `json:"bytes_in"`
	BytesOut      int64     `json:"bytes_out"`
	RequestID     string    `json:"request_id"`
	TraceID       string    `json:"trace_id,omitempty"`
	UpstreamError bool      `json:"upstream_error"`
	ColdStart     bool      `json:"cold_start"`
	VisitorID     string    `json:"visitor_id,omitempty"`
	ReferrerHost  string    `json:"referrer_host,omitempty"`
	UAFamily      string    `json:"ua_family,omitempty"`
	ClientIP      string    `json:"client_ip,omitempty"`
	UserAgent     string    `json:"user_agent,omitempty"`
}

// NewEvent builds an Event from an access record and the app name the caller
// resolved for its route key.
func NewEvent(app string, rec ingress.AccessRecord) Event {
	return Event{
		Time: rec.Time, App: app, Host: rec.Host, Method: rec.Method, Path: rec.Path,
		Status: rec.Status, DurationMS: rec.Millis, BytesIn: rec.BytesIn, BytesOut: rec.BytesOut,
		RequestID: rec.RequestID, TraceID: rec.TraceID, UpstreamError: rec.Upstream, ColdStart: rec.Cold,
	}
}
