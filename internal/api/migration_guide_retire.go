package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/datamigrate"
	"github.com/GLINCKER/levelrail/internal/migrationgate"
	"github.com/GLINCKER/levelrail/internal/store"
)

const envMigrateProbeTimeout = "APP_MIGRATE_PROBE_TIMEOUT"

func (rt *Router) migrationProberOrDefault() datamigrate.Prober {
	if rt.migrationProber != nil {
		return *rt.migrationProber
	}
	return datamigrate.Prober{Resolver: rt.migrationResolver(), Timeout: durationEnv(envMigrateProbeTimeout, 8*time.Second)}
}

type migrationProbeRequest struct {
	Domain string `json:"domain"`
	// Kind is "dns", "http" or empty for both.
	Kind string `json:"kind,omitempty"`
}

type migrationProbeResult struct {
	Domain  string              `json:"domain"`
	App     string              `json:"app"`
	Targets []string            `json:"targets"`
	DNS     *datamigrate.Check  `json:"dns,omitempty"`
	HTTP    []datamigrate.Check `json:"http,omitempty"`
	OK      bool                `json:"ok"`
}

func healthPathOf(it store.AppImportItem) string {
	if rec := decodeAppImportRecord(it.EntryJSON); rec.Entry.Health != nil && rec.Entry.Health.Path != "" {
		return rec.Entry.Health.Path
	}
	return "/"
}

func ownerOfDomain(items []store.AppImportItem, domain string) (store.AppImportItem, bool) {
	for _, it := range items {
		if !it.Selected {
			continue
		}
		for _, d := range it.Domains {
			if trimmedLower(d) == trimmedLower(domain) {
				return it, true
			}
		}
	}
	return store.AppImportItem{}, false
}

func checksPass(cs ...datamigrate.Check) bool {
	for _, c := range cs {
		if c.Status == datamigrate.CheckFail {
			return false
		}
	}
	return true
}

func (rt *Router) probeDomain(r *http.Request, it store.AppImportItem, domain, kind string, targets []string) migrationProbeResult {
	prober := rt.migrationProberOrDefault()
	res := migrationProbeResult{Domain: domain, App: it.TargetName, Targets: targets, OK: true}
	if kind == "" || kind == "dns" {
		c := prober.Resolves(r.Context(), domain, targets)
		res.DNS = &c
		res.OK = res.OK && checksPass(c)
	}
	if kind == "" || kind == "http" {
		res.HTTP = prober.Responds(r.Context(), domain, healthPathOf(it), targets)
		res.OK = res.OK && checksPass(res.HTTP...)
	}
	return res
}

func trimmedLower(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// handleAppImportProbe handles POST .../sessions/{id}/probe: the per-domain "check DNS now" and
// "does the app answer on this server" checks. Only a domain of a selected app can be probed, so
// the route cannot be used to scan arbitrary hosts.
func (rt *Router) handleAppImportProbe(w http.ResponseWriter, r *http.Request) {
	_, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	var req migrationProbeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAppImportBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Kind != "" && req.Kind != "dns" && req.Kind != "http" {
		writeError(w, http.StatusBadRequest, "kind must be dns, http or empty")
		return
	}
	it, found := ownerOfDomain(items, req.Domain)
	if !found {
		writeError(w, http.StatusNotFound, "the domain does not belong to a selected app of this job")
		return
	}
	targets, bad := rt.migrationTargetIPs(r.Context(), r)
	if bad != "" {
		writeError(w, http.StatusBadRequest, bad)
		return
	}
	writeJSON(w, http.StatusOK, rt.probeDomain(r, it, req.Domain, req.Kind, targets))
}

type migrationRetireRequest struct {
	// Probe runs the live DNS and response checks. Without it those gates stay pending.
	Probe        bool     `json:"probe,omitempty"`
	Acknowledged []string `json:"acknowledged,omitempty"`
}

func (rt *Router) retireInput(r *http.Request, items []store.AppImportItem, req migrationRetireRequest) migrationgate.Input {
	in := migrationgate.Input{Acknowledged: req.Acknowledged}
	dbSeen := map[string]bool{}
	for _, it := range items {
		rec := decodeAppImportRecord(it.EntryJSON)
		if !it.Selected {
			if isUnsupportedEntry(rec.Entry) {
				in.Items = append(in.Items, migrationgate.Item{Name: it.SourceName, Leftover: true})
			}
			continue
		}
		gi := migrationgate.Item{Name: it.SourceName, State: it.State, Domains: it.Domains}
		if interrupted(it, rt.appImportLive.isRunning(it.SessionID)) {
			gi.State = store.AppImportVerifyFailed
		}
		for _, v := range it.Volumes {
			gi.Volumes = append(gi.Volumes, migrationgate.Volume{Name: v.Name, Copied: v.Copied})
		}
		in.Items = append(in.Items, gi)
		for _, d := range rec.Entry.Databases {
			key := d.Target
			if key == "" {
				key = "source:" + d.SourceID
			}
			if dbSeen[key] {
				continue
			}
			dbSeen[key] = true
			gd := migrationgate.Database{Name: firstNonEmpty(d.Target, d.Name)}
			if d.Target != "" {
				if row, found, err := rt.dataImports.GetDatabaseDataImport(r.Context(), d.Target); err == nil && found {
					gd.Status, gd.Mismatched = row.Status, row.Mismatched
				}
			}
			in.Databases = append(in.Databases, gd)
		}
	}
	if req.Probe {
		in.Probes = rt.retireProbes(r, items)
	}
	return in
}

func (rt *Router) retireProbes(r *http.Request, items []store.AppImportItem) []migrationgate.Probe {
	targets, _ := rt.migrationTargetIPs(r.Context(), r)
	type job struct {
		it     store.AppImportItem
		domain string
	}
	var jobs []job
	for _, it := range items {
		if !it.Selected || len(it.Domains) == 0 {
			continue
		}
		for _, d := range it.Domains {
			jobs = append(jobs, job{it, d})
		}
	}
	out := make([]migrationgate.Probe, len(jobs))
	rt.parallel(len(jobs), func(i int) {
		res := rt.probeDomain(r, jobs[i].it, jobs[i].domain, "", targets)
		out[i] = migrationgate.Probe{Domain: jobs[i].domain, DNS: res.DNS != nil && checksPass(*res.DNS), Responds: checksPass(res.HTTP...)}
	})
	return out
}

// handleAppImportRetire serves both verbs on .../sessions/{id}/retire. GET reads recorded evidence
// only and never touches the network; POST can run the live probes and take acknowledgments.
func (rt *Router) handleAppImportRetire(w http.ResponseWriter, r *http.Request) {
	_, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	var req migrationRetireRequest
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAppImportBody)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	writeJSON(w, http.StatusOK, migrationgate.Evaluate(rt.retireInput(r, items, req)))
}
