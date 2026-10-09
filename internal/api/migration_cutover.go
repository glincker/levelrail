package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/datamigrate"
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

const cutoverParallelism = 6

type cutoverReport struct {
	Verdict   string                   `json:"verdict"`
	TargetIPs []string                 `json:"target_ips"`
	Domains   []datamigrate.DomainPlan `json:"domains"`
	Guidance  []string                 `json:"guidance"`
	Phase     string                   `json:"phase"`
}

type cutoverDomain struct {
	app, domain, healthPath string
	ready                   bool
	readyDetail             string
}

func (rt *Router) migrationResolver() datamigrate.Resolver {
	if rt.dnsResolver != nil {
		return rt.dnsResolver
	}
	return datamigrate.DNSResolver{}
}

// importedDomains returns one entry per domain of every imported app, with
// whether the app is healthy here right now.
func (rt *Router) importedDomains(ctx context.Context, only string) ([]cutoverDomain, error) {
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, err
	}
	var out []cutoverDomain
	for _, s := range svcs {
		if s.Labels[platformimport.LabelSourceID] == "" || (only != "" && s.Name != only) {
			continue
		}
		ready, detail := rt.appReadiness(ctx, s)
		path := "/"
		if s.Health != nil && s.Health.Readiness != nil && s.Health.Readiness.Path != "" {
			path = s.Health.Readiness.Path
		}
		for _, d := range s.Domains {
			out = append(out, cutoverDomain{app: s.Name, domain: d, healthPath: path, ready: ready, readyDetail: detail})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].app != out[j].app {
			return out[i].app < out[j].app
		}
		return out[i].domain < out[j].domain
	})
	return out, nil
}

func (rt *Router) appReadiness(ctx context.Context, s store.DesiredService) (bool, string) {
	conds, err := rt.deploys.GetConditions(ctx, applicationControllerName(s.Name))
	if err != nil {
		rt.logger.Warn("api: cutover: read app conditions failed", slog.String("error", err.Error()), slog.String("app", s.Name))
		return false, "status unavailable"
	}
	sum := summarizeAppConditions(conds)
	if sum.Variant == "success" {
		return true, sum.Label
	}
	for _, c := range conds {
		if c.Status == reconcile.ConditionFalse && c.Message != "" {
			return false, sum.Label + " (" + c.Message + ")"
		}
	}
	return false, sum.Label
}

func (rt *Router) migrationTargetIPs(ctx context.Context, r *http.Request) ([]string, string) {
	if v := strings.TrimSpace(r.URL.Query().Get("target_ip")); v != "" {
		if _, err := netip.ParseAddr(v); err != nil {
			return nil, "target_ip is not a valid IP address"
		}
		return []string{v}, ""
	}
	endpoint := rt.doctorPublicIPEndpoint
	if endpoint == "" {
		endpoint = defaultDoctorPublicIPEndpoint
	}
	ip, err := doctorFetchPublicIP(ctx, rt.doctorHTTPClientOrDefault(), endpoint)
	if err != nil {
		return nil, ""
	}
	return []string{ip}, ""
}

// handleCutoverReport handles GET /api/v1/migration/cutover. Read-only: it
// reads DNS and app status and changes nothing.
func (rt *Router) handleCutoverReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	targets, bad := rt.migrationTargetIPs(ctx, r)
	if bad != "" {
		writeError(w, http.StatusBadRequest, bad)
		return
	}
	doms, err := rt.importedDomains(ctx, r.URL.Query().Get("app"))
	if err != nil {
		rt.internalError(w, "api: cutover report: list apps failed", err)
		return
	}
	resolver := rt.migrationResolver()
	plans := make([]datamigrate.DomainPlan, len(doms))
	rt.parallel(len(doms), func(i int) {
		d := doms[i]
		plans[i] = datamigrate.Evaluate(datamigrate.EvalInput{App: d.app, Domain: d.domain, Ready: d.ready, ReadyDetail: d.readyDetail,
			DNS: resolver.Lookup(ctx, d.domain), TargetIPs: targets})
	})
	writeJSON(w, http.StatusOK, cutoverReport{Verdict: datamigrate.Overall(plans), TargetIPs: targets, Domains: plans, Phase: "pre-switch",
		Guidance: cutoverGuidance(len(targets) > 0)})
}

// handleCutoverVerify handles GET /api/v1/migration/cutover/verify.
func (rt *Router) handleCutoverVerify(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	targets, bad := rt.migrationTargetIPs(ctx, r)
	if bad != "" {
		writeError(w, http.StatusBadRequest, bad)
		return
	}
	doms, err := rt.importedDomains(ctx, r.URL.Query().Get("app"))
	if err != nil {
		rt.internalError(w, "api: cutover verify: list apps failed", err)
		return
	}
	prober := datamigrate.Prober{Resolver: rt.migrationResolver()}
	plans := make([]datamigrate.DomainPlan, len(doms))
	rt.parallel(len(doms), func(i int) {
		d := doms[i]
		checks := prober.PostSwitch(ctx, d.domain, d.healthPath, targets)
		verdict := datamigrate.VerdictSwitch
		for _, c := range checks {
			if c.Status == datamigrate.CheckFail {
				verdict = datamigrate.VerdictNoGo
			}
		}
		plans[i] = datamigrate.DomainPlan{App: d.app, Domain: d.domain, Verdict: verdict, Checks: checks}
	})
	writeJSON(w, http.StatusOK, cutoverReport{Verdict: datamigrate.Overall(plans), TargetIPs: targets, Domains: plans, Phase: "post-switch",
		Guidance: []string{"Keep the source running until every domain shows switched here, then stop it only after you are happy with a full day of traffic."}})
}

func (rt *Router) parallel(n int, fn func(i int)) {
	sem := make(chan struct{}, cutoverParallelism)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			fn(i)
		}(i)
	}
	wg.Wait()
}

func cutoverGuidance(haveIP bool) []string {
	g := []string{
		"The source keeps serving traffic until you change DNS. Nothing here switches it for you.",
		"Lower each domain's TTL to 300 seconds first and wait out the old TTL, so the switch takes effect within minutes and can be reverted as fast.",
		"Change one domain at a time, then run the post-switch check. To roll back, point the record at the source again.",
	}
	if !haveIP {
		g = append(g, "This node's public IP could not be detected. Pass the IP to check against explicitly.")
	}
	return g
}

type volumeGuideResponse struct {
	Source  string                    `json:"source"`
	Guides  []datamigrate.VolumeGuide `json:"guides"`
	Warning string                    `json:"warning"`
}

// handleVolumeGuide handles GET /api/v1/migration/volumes?source=user@host. It
// only prints commands: moving volume contents needs root on the source.
func (rt *Router) handleVolumeGuide(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	if err := datamigrate.ValidateSSHTarget(source); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		rt.internalError(w, "api: volume guide: list apps failed", err)
		return
	}
	only := r.URL.Query().Get("app")
	guides := []datamigrate.VolumeGuide{}
	for _, s := range svcs {
		if s.Labels[platformimport.LabelSourceID] == "" || (only != "" && s.Name != only) {
			continue
		}
		var vols []datamigrate.VolumeRef
		for _, v := range s.Volumes {
			vols = append(vols, datamigrate.VolumeRef{Name: v.Name, ContainerPath: v.ContainerPath})
		}
		for _, b := range s.BindMounts {
			vols = append(vols, datamigrate.VolumeRef{HostPath: b.HostPath, ContainerPath: b.ContainerPath})
		}
		guides = append(guides, datamigrate.GuideVolumes(s.Name, "app-"+s.Name+"-", source, vols)...)
	}
	writeJSON(w, http.StatusOK, volumeGuideResponse{Source: source, Guides: guides,
		Warning: "Run these as root on the node that hosts the app, with the app stopped or between writes. Run them once while the source is live to move the bulk, then again right before the DNS switch to catch the last changes."})
}
