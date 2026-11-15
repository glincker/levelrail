package api

import (
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// egressReportName is one name an app asked for, with whether its allowlist would refuse it.
type egressReportName struct {
	Name      string    `json:"name"`
	Count     int       `json:"count"`
	LastSeen  time.Time `json:"last_seen"`
	WouldDeny bool      `json:"would_deny"`
	Reason    string    `json:"reason,omitempty"`
}

// egressReportResponse is the body of GET /api/v1/apps/{name}/egress-report (read only, beta).
type egressReportResponse struct {
	App       string             `json:"app"`
	Enabled   bool               `json:"enabled"`
	Privacy   string             `json:"privacy"`
	Mode      string             `json:"mode,omitempty"`
	Allow     []string           `json:"allow,omitempty"`
	Message   string             `json:"message,omitempty"`
	Names     []egressReportName `json:"names"`
	Truncated int                `json:"truncated,omitempty"`
}

// egressHostMatches compares a queried name with one allowlist host. Hosts are
// exact today; a leading "*." is accepted so a wildcard policy needs no report change.
func egressHostMatches(allowed, qname string) bool {
	allowed = strings.ToLower(strings.TrimSuffix(allowed, "."))
	qname = strings.ToLower(strings.TrimSuffix(qname, "."))
	if suffix, ok := strings.CutPrefix(allowed, "*."); ok {
		return strings.HasSuffix(qname, "."+suffix)
	}
	return allowed == qname
}

// egressMeshReason reports whether the mesh server answered from its own zone, which is not egress.
func egressMeshReason(reason string) bool {
	switch reason {
	case network.ReasonLocalRecord, network.ReasonNoData, network.ReasonNoSuchApp:
		return true
	}
	return false
}

// buildEgressReport folds log records for one app into per name rows, newest name first.
func buildEgressReport(records []network.QueryRecord, allow []string, maxNames int) (rows []egressReportName, truncated int) {
	byName := map[string]*egressReportName{}
	for _, r := range records {
		if egressMeshReason(r.Reason) {
			continue
		}
		name := strings.TrimSuffix(r.QName, ".")
		row := byName[name]
		if row == nil {
			row = &egressReportName{Name: name, WouldDeny: true, Reason: "not in egress.allow"}
			for _, a := range allow {
				if egressHostMatches(a, name) {
					row.WouldDeny, row.Reason = false, ""
					break
				}
			}
			byName[name] = row
		}
		row.Count++
		if r.Time.After(row.LastSeen) {
			row.LastSeen = r.Time
		}
	}
	rows = make([]egressReportName, 0, len(byName))
	for _, row := range byName {
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].WouldDeny != rows[j].WouldDeny {
			return rows[i].WouldDeny
		}
		if !rows[i].LastSeen.Equal(rows[j].LastSeen) {
			return rows[i].LastSeen.After(rows[j].LastSeen)
		}
		return rows[i].Name < rows[j].Name
	})
	if maxNames > 0 && len(rows) > maxNames {
		truncated = len(rows) - maxNames
		rows = rows[:maxNames]
	}
	return rows, truncated
}

func (rt *Router) handleAppEgressReport(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: egress report: load app failed", err, slog.String("name", name))
		return
	}
	out := egressReportResponse{App: name, Enabled: rt.dnsQueryLog.Enabled(), Privacy: rt.dnsQueryLog.Privacy(), Names: []egressReportName{}}
	switch {
	case !out.Enabled:
		out.Message = "dns query log is off (set dns.query_log_enabled to true)"
	case out.Privacy == network.QueryPrivacyCountersOnly:
		out.Message = "no data: privacy is counters_only, so no names are stored"
	case svc.Egress == nil:
		out.Message = "this app has no egress allowlist, so nothing would be denied"
	}
	if svc.Egress != nil {
		out.Mode = svc.Egress.Mode
		for _, a := range svc.Egress.Allow {
			out.Allow = append(out.Allow, a.Host)
		}
	}
	if out.Message == "" {
		maxNames := 0
		if rt.runtimeSettings != nil {
			maxNames = rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyDNSEgressReportMaxNames)
		}
		out.Names, out.Truncated = buildEgressReport(rt.dnsQueryLog.Query(network.QueryFilter{App: name}), out.Allow, maxNames)
	}
	writeJSON(w, http.StatusOK, out)
}
