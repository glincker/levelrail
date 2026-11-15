package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// dnsQueriesDefaultLimit is the page size when the caller sends no limit.
const dnsQueriesDefaultLimit = 100

// dnsQueriesResponse is the body of GET /api/v1/dns/queries (beta).
type dnsQueriesResponse struct {
	Enabled bool                  `json:"enabled"`
	Privacy string                `json:"privacy"`
	Queries []network.QueryRecord `json:"queries"`
}

// dnsStatsResponse is the body of GET /api/v1/dns/stats (beta).
type dnsStatsResponse struct {
	Enabled bool                     `json:"enabled"`
	Privacy string                   `json:"privacy"`
	Apps    []network.AppDNSCounters `json:"apps"`
	Total   network.AppDNSCounters   `json:"total"`
}

// SetDNSQueryLog wires the mesh resolver's query log; nil leaves both routes reporting disabled.
func (rt *Router) SetDNSQueryLog(l *network.QueryLog) { rt.dnsQueryLog = l }

func (rt *Router) handleListDNSQueries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, err := queryInt(q.Get("limit"), dnsQueriesDefaultLimit)
	if err != nil || limit < 1 {
		writeError(w, http.StatusBadRequest, "limit must be a positive integer")
		return
	}
	limit = min(limit, runtimesettings.DNSQueryLogSizeMax)
	f := network.QueryFilter{
		App: q.Get("app"), Prefix: strings.TrimSpace(q.Get("qname")), RCode: q.Get("rcode"), Limit: limit,
	}
	if raw := q.Get("since"); raw != "" {
		since, ok := parseDNSSince(raw, time.Now())
		if !ok {
			writeError(w, http.StatusBadRequest, "since must be an RFC 3339 time or a duration such as 15m")
			return
		}
		f.Since = since
	}
	writeJSON(w, http.StatusOK, dnsQueriesResponse{
		Enabled: rt.dnsQueryLog.Enabled(), Privacy: rt.dnsQueryLog.Privacy(), Queries: rt.dnsQueryLog.Query(f),
	})
}

func (rt *Router) handleDNSStats(w http.ResponseWriter, _ *http.Request) {
	apps, total := rt.dnsQueryLog.Stats()
	writeJSON(w, http.StatusOK, dnsStatsResponse{
		Enabled: rt.dnsQueryLog.Enabled(), Privacy: rt.dnsQueryLog.Privacy(), Apps: apps, Total: total,
	})
}

// parseDNSSince accepts an RFC 3339 time or a lookback duration.
func parseDNSSince(raw string, now time.Time) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, true
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return now.Add(-d), true
	}
	return time.Time{}, false
}
