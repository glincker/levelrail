package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/cutover"
	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/domaindoctor"
	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/libdns/libdns"
)

// cutoverBackend implements every cutover.Deps interface on the router.
// It never receives the source credential or a source client.
type cutoverBackend struct{ rt *Router }

func (rt *Router) cutoverDeps() cutover.Deps {
	b := cutoverBackend{rt: rt}
	return cutover.Deps{Planner: b, Host: b, Ingress: b, DNS: b, Proxy: b, Verifier: b}
}

// findCutoverItem returns the import item whose staged app is app, or whose
// domains include domain when app is empty.
func (b cutoverBackend) findItem(ctx context.Context, app, domain string) (store.AppImportItem, bool) {
	sessions, err := b.rt.appImports.ListAppImportSessions(ctx)
	if err != nil {
		return store.AppImportItem{}, false
	}
	for _, s := range sessions {
		items, err := b.rt.appImports.ListAppImportItems(ctx, s.ID)
		if err != nil {
			continue
		}
		for _, it := range items {
			if !it.Selected {
				continue
			}
			if (app != "" && it.TargetName == app) || (app == "" && domain != "" && slices.ContainsFunc(it.Domains, func(d string) bool { return strings.EqualFold(d, domain) })) {
				return it, true
			}
		}
	}
	return store.AppImportItem{}, false
}

func recordsOfRR(rrs []dnsRecordResource) []cutover.Record {
	out := make([]cutover.Record, 0, len(rrs))
	for _, r := range rrs {
		out = append(out, cutover.Record{Name: r.Name, Type: r.Type, Value: r.Value, TTL: r.TTLSeconds})
	}
	return out
}

// Plan implements cutover.Planner.
func (b cutoverBackend) Plan(ctx context.Context, req cutover.PlanRequest) (cutover.Plan, error) {
	rt := b.rt
	items, err := rt.appImports.ListAppImportItems(ctx, req.SessionID)
	if err != nil {
		return cutover.Plan{}, fmt.Errorf("load import items: %w", err)
	}
	idx := findItem(items, req.SourceID)
	if idx < 0 {
		return cutover.Plan{}, errors.New("the import item no longer exists")
	}
	it := items[idx]
	rec := decodeAppImportRecord(it.EntryJSON)
	f := cutover.Facts{App: it.TargetName, ItemState: it.State, Now: time.Now()}
	b.imageFacts(ctx, req.SessionID, items, it, rec, &f)
	f.EnvPlain, f.EnvSecret, f.EnvEmptySecrets = rec.Entry.Env.Plain, rec.Entry.Env.Secret, rec.Entry.Env.Empty
	f.Databases = b.databaseFacts(ctx, rec)
	for _, v := range it.Volumes {
		f.Volumes = append(f.Volumes, cutover.VolumeFact{Name: v.Name, Path: v.ContainerPath, Copied: v.Copied})
	}
	b.healthFacts(ctx, it.TargetName, rec, &f)
	f.Domains = b.domainFacts(ctx, it, req.DNSWrite)
	return cutover.Evaluate(f), nil
}

func (b cutoverBackend) imageFacts(ctx context.Context, sessionID string, items []store.AppImportItem, it store.AppImportItem, rec appImportRecord, f *cutover.Facts) {
	f.Image = rec.Entry.Image
	switch {
	case rec.Entry.HostBuilt:
		f.ImageSource = cutover.ImageHostBuilt
		for _, img := range b.rt.imagesView(ctx, sessionID, items, true).Images {
			if img.SourceID == it.SourceID {
				f.ImageVerified = img.Verified
				switch {
				case img.Verified:
					f.ImageDetail = fmt.Sprintf("%s is on this node and its content matches the source (%s)", img.Image, shortID(img.LoadedImageID))
				case img.Error != "":
					f.ImageDetail = img.Image + ": " + img.Error
				}
			}
		}
	case rec.Entry.Source == appimportSourceGit:
		f.ImageSource = cutover.ImageGit
	default:
		f.ImageSource = cutover.ImageRegistry
	}
}

const appimportSourceGit = "git"

func shortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func (b cutoverBackend) databaseFacts(ctx context.Context, rec appImportRecord) []cutover.DBFact {
	var out []cutover.DBFact
	verified := map[string]store.MigrationItem{}
	if b.rt.migrationHub != nil {
		if sessions, err := b.rt.migrationHub.ListMigrationSessions(ctx); err == nil {
			for _, s := range sessions {
				hubItems, err := b.rt.migrationHub.ListMigrationItems(ctx, s.ID)
				if err != nil {
					continue
				}
				for _, hi := range hubItems {
					if hi.Status == store.HubItemVerified && hi.TargetName != "" {
						verified[strings.ToLower(hi.TargetName)] = hi
					}
				}
			}
		}
	}
	for _, d := range rec.Entry.Databases {
		fact := cutover.DBFact{Name: d.Name, Host: d.Host, Target: d.Target}
		if hi, ok := verified[strings.ToLower(d.Target)]; ok && d.Target != "" {
			fact.Verified, fact.Checked, fact.Mismatched = true, hi.Checked, hi.Mismatched
		}
		out = append(out, fact)
	}
	return out
}

func (b cutoverBackend) healthFacts(ctx context.Context, app string, rec appImportRecord, f *cutover.Facts) {
	if svc, err := b.rt.apps.GetDesiredService(ctx, app); err == nil && svc != nil && svc.Health != nil && svc.Health.Readiness != nil && svc.Health.Readiness.Path != "" {
		f.HealthKnown, f.HealthPath = true, svc.Health.Readiness.Path
		return
	}
	if h := rec.Entry.Health; h != nil && h.Path != "" {
		f.HealthKnown, f.HealthPath = true, h.Path
	}
}

func (b cutoverBackend) domainFacts(ctx context.Context, it store.AppImportItem, dnsWrite bool) []cutover.DomainFact {
	rt := b.rt
	ips := rt.serverPublicIPs(ctx)
	resolver := rt.migrationResolver()
	taken := b.domainHolders(ctx, it.TargetName)
	proxyOn := rt.proxyCoexistActive(ctx)
	out := make([]cutover.DomainFact, 0, len(it.Domains))
	for _, domain := range it.Domains {
		st := resolver.Lookup(ctx, domain)
		fact := cutover.DomainFact{Domain: domain, TakenBy: taken[strings.ToLower(domain)]}
		fact.Current = append(fact.Current, st.Addrs...)
		if st.CNAME != "" {
			fact.Current = append(fact.Current, "CNAME "+strings.TrimSuffix(st.CNAME, "."))
		}
		for _, a := range st.Addrs {
			if slices.Contains(ips, a) {
				fact.PointsHere = true
			}
		}
		fact.Plan = b.planDomain(ctx, it.TargetName, domain, ips, dnsWrite, proxyOn, &fact)
		out = append(out, fact)
	}
	return out
}

func (b cutoverBackend) domainHolders(ctx context.Context, app string) map[string]string {
	out := map[string]string{}
	svcs, err := b.rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return out
	}
	for _, s := range svcs {
		if s.Name == app {
			continue
		}
		for _, d := range s.Domains {
			out[strings.ToLower(d)] = s.Name
		}
	}
	return out
}

func (b cutoverBackend) planDomain(ctx context.Context, app, domain string, ips []string, dnsWrite, proxyOn bool, fact *cutover.DomainFact) cutover.DomainPlan {
	plan := cutover.DomainPlan{Domain: domain, Current: fact.Current, Method: cutover.MethodManual}
	res, _ := b.rt.applyDomainDNS(ctx, nil, app, domain, dnsOpts{Mode: dnsModePreview, Replace: true})
	plan.Provider, plan.Zone = res.Provider, res.Zone
	if res.Record != nil {
		plan.Desired = &cutover.Record{Name: res.Record.Name, Type: res.Record.Type, Value: res.Record.Value, TTL: res.Record.TTLSeconds}
	} else if typ, val, err := dnsrecords.TargetFor(ips); err == nil {
		plan.Desired = &cutover.Record{Name: domain, Type: typ, Value: val}
	}
	plan.Replace = recordsOfRR(res.Replaced)
	switch {
	case res.Planned == dnsrecords.OutcomeUnchanged:
		fact.PointsHere = true
	case res.DNS == dnsResultPreview && res.Provider != "" && dnsWrite && res.Planned != dnsrecords.OutcomeConflict:
		plan.Method = cutover.MethodDNS
	case proxyOn:
		plan.Method = cutover.MethodProxy
	default:
		switch {
		case res.Provider != "" && !dnsWrite:
			plan.Message = "changing DNS records needs the root ability"
		case res.Message != "":
			plan.Message = res.Message
		}
	}
	return plan
}

// proxyCoexistActive reports whether the managed proxy integration is on.
func (rt *Router) proxyCoexistActive(ctx context.Context) bool {
	if rt.proxyIntegration == nil || rt.proxyIntegration.store == nil {
		return false
	}
	s, err := rt.proxyIntegration.store.GetProxyIntegrationSettings(ctx)
	return err == nil && s.Enabled()
}

// Route implements cutover.Host: attach the domains and start the app.
func (b cutoverBackend) Route(ctx context.Context, app string, _ []string) error {
	it, ok := b.findItem(ctx, app, "")
	if !ok {
		return errors.New("the import item no longer exists")
	}
	editor, ok := b.rt.apps.(serviceDomainsEditor)
	if !ok {
		return errors.New("domain editing is not available on this control plane")
	}
	if err := b.rt.enableAppImportRouting(ctx, editor, &it, routeAppImportRequest{Enable: true, IgnoreVolumes: true}); err != nil {
		return err
	}
	b.rt.saveAppImportItem(ctx, it)
	b.rt.nudgeReconciler()
	return nil
}

// Unroute implements cutover.Host: detach the domains and stop the app.
func (b cutoverBackend) Unroute(ctx context.Context, app string, _ []string) error {
	it, ok := b.findItem(ctx, app, "")
	if !ok || it.State != store.AppImportRouted {
		return nil
	}
	editor, ok := b.rt.apps.(serviceDomainsEditor)
	if !ok {
		return errors.New("domain editing is not available on this control plane")
	}
	if err := b.rt.disableAppImportRouting(ctx, editor, &it); err != nil {
		return err
	}
	b.rt.saveAppImportItem(ctx, it)
	b.rt.nudgeReconciler()
	return nil
}

// WaitHealthy implements cutover.Host.
func (b cutoverBackend) WaitHealthy(ctx context.Context, app string, timeout time.Duration) (bool, string, error) {
	svc, err := b.rt.apps.GetDesiredService(ctx, app)
	if err != nil {
		return false, "", fmt.Errorf("load the app: %w", err)
	}
	deadline := time.Now().Add(timeout)
	for {
		ready, detail := b.rt.appReadiness(ctx, *svc)
		if ready {
			return true, detail, nil
		}
		if time.Now().After(deadline) {
			return false, "not ready within " + timeout.String() + ": " + detail, nil
		}
		select {
		case <-ctx.Done():
			return false, "", ctx.Err()
		case <-time.After(appImportReadyPoll):
		}
	}
}

// Probe implements cutover.Ingress: a request through this node's own
// ingress listener, HTTPS first and plain HTTP while no certificate exists.
func (b cutoverBackend) Probe(ctx context.Context, domain, path string) (cutover.ProbeResult, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	probe := b.rt.cutover.httpProbe
	if probe == nil {
		probe = defaultHTTPProbe
	}
	httpsAddr := b.rt.probeDialAddr(domain, false, 0)
	status, err := probe(ctx, "https://"+domain+path, httpsAddr)
	if err == nil {
		return cutover.ProbeResult{OK: probeStatusOK(status), Status: status, Detail: "https answered " + strconv.Itoa(status)}, nil
	}
	port := b.rt.doctorHTTPPort
	if port == 0 {
		port = defaultDoctorHTTPPort
	}
	hstatus, herr := probe(ctx, "http://"+domain+path, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if herr != nil {
		return cutover.ProbeResult{Detail: "https: " + err.Error() + "; http: " + herr.Error()}, nil
	}
	return cutover.ProbeResult{OK: probeStatusOK(hstatus), Status: hstatus,
		Detail: fmt.Sprintf("http answered %d (no certificate yet, expected until DNS points here)", hstatus)}, nil
}

func probeStatusOK(status int) bool {
	return (status >= 200 && status < 400) || status == 401 || status == 403
}

// Apply implements cutover.DNS through the same writer go-live uses.
func (b cutoverBackend) Apply(ctx context.Context, domain string) (cutover.DNSResult, error) {
	it, ok := b.findItem(ctx, "", domain)
	if !ok {
		return cutover.DNSResult{}, errors.New("the import item no longer exists")
	}
	res, _ := b.rt.applyDomainDNS(ctx, nil, it.TargetName, domain, dnsOpts{Mode: dnsModeAuto, Replace: true, CanWrite: true})
	switch res.DNS {
	case dnsrecords.OutcomeCreated, dnsrecords.OutcomeUpdated, dnsrecords.OutcomeUnchanged:
	default:
		return cutover.DNSResult{}, fmt.Errorf("DNS was not changed (%s): %s", res.DNS, res.Message)
	}
	out := cutover.DNSResult{Provider: res.Provider, Zone: res.Zone, Message: res.Message, Previous: recordsOfRR(res.Replaced)}
	if res.Record != nil {
		out.Applied = &cutover.Record{Name: res.Record.Name, Type: res.Record.Type, Value: res.Record.Value, TTL: res.Record.TTLSeconds}
	}
	return out, nil
}

// Restore implements cutover.DNS: remove what was written and put the
// previous records back, both idempotent.
func (b cutoverBackend) Restore(ctx context.Context, domain string, previous []cutover.Record, applied *cutover.Record) error {
	t, err := b.rt.resolveDNSTarget(ctx, domain)
	if err != nil || t == nil {
		return errors.New("the DNS provider is not reachable")
	}
	if applied != nil {
		want := libdns.RR{Name: applied.Name, Type: applied.Type, Data: applied.Value}
		if err := deleteExactRecord(ctx, t, want); err != nil {
			return fmt.Errorf("remove the new record: %w", err)
		}
		if ms := b.rt.managedDNS(); ms != nil {
			_ = ms.DeleteManagedDNSRecord(ctx, domain, applied.Type)
		}
		b.rt.recordDNSAudit(ctx, nil, store.AuditActionDNSRecordDeleted, "", domain, want)
	}
	if len(previous) == 0 {
		return nil
	}
	live, err := t.Manager.GetRecords(ctx, t.Zone)
	if err != nil {
		return fmt.Errorf("read the zone: %w", err)
	}
	for _, p := range previous {
		rr := libdns.RR{Name: p.Name, Type: p.Type, Data: p.Value, TTL: time.Duration(p.TTL) * time.Second}
		if slices.ContainsFunc(live, func(r libdns.Record) bool {
			x := r.RR()
			return strings.EqualFold(x.Name, rr.Name) && x.Type == rr.Type && strings.EqualFold(strings.TrimSuffix(x.Data, "."), strings.TrimSuffix(rr.Data, "."))
		}) {
			continue
		}
		if _, err := t.Manager.AppendRecords(ctx, t.Zone, []libdns.Record{rr}); err != nil {
			return fmt.Errorf("restore %s %s: %w", p.Type, p.Name, err)
		}
		b.rt.recordDNSAudit(ctx, nil, store.AuditActionDNSRecordCreated, "", domain, rr)
	}
	return nil
}

// WriteRoute implements cutover.Proxy: the route is derived from the
// attached domains, so this syncs and confirms it was written.
func (b cutoverBackend) WriteRoute(ctx context.Context, domain string) error {
	deps := b.rt.proxyIntegration
	if deps == nil || deps.syncer == nil {
		return errors.New("managed proxy routes are not available on this control plane")
	}
	b.rt.syncProxyRoutes(ctx, deps)
	plan, err := deps.syncer.Plan(ctx)
	if err != nil {
		return fmt.Errorf("read the proxy plan: %w", err)
	}
	for _, it := range plan.Items {
		if strings.EqualFold(it.Domain, domain) {
			if st, _ := plan.State(it); st != proxyroutes.StateWritten {
				return fmt.Errorf("the proxy route for %s is %s, not written", domain, st)
			}
			return nil
		}
	}
	return fmt.Errorf("no proxy route is planned for %s", domain)
}

// RemoveRoute implements cutover.Proxy by detaching the domain and syncing.
func (b cutoverBackend) RemoveRoute(ctx context.Context, domain string) error {
	if it, ok := b.findItem(ctx, "", domain); ok {
		if editor, ok := b.rt.apps.(serviceDomainsEditor); ok {
			_, _, err := editor.EditServiceDomains(ctx, it.TargetName, func(cur []string) ([]string, error) {
				return applyDomainEdit(cur, nil, []string{domain})
			})
			if err != nil && !errors.Is(err, errDomainNotSet) {
				return fmt.Errorf("detach %s: %w", domain, err)
			}
		}
	}
	if deps := b.rt.proxyIntegration; deps != nil && deps.syncer != nil {
		b.rt.syncProxyRoutes(ctx, deps)
	}
	return nil
}

// Verify implements cutover.Verifier with the domain doctor.
func (b cutoverBackend) Verify(ctx context.Context, app, domain string) (bool, string, error) {
	rt := b.rt
	svc, err := rt.apps.GetDesiredService(ctx, app)
	if err != nil {
		return false, "", fmt.Errorf("load the app: %w", err)
	}
	held := rt.heldBackDomains(ctx)
	probes := domaindoctor.DefaultProbes()
	if rt.traffic != nil && rt.traffic.doctorProbes != nil {
		probes = *rt.traffic.doctorProbes
	}
	rep := domaindoctor.Run(ctx, rt.doctorFacts(ctx, svc, domain, held), probes, domaindoctor.Options{
		Timeout:        envDuration(envDomainDoctorTimeout, defaultDomainDoctorTimeout),
		ProbeTimeout:   envDuration(envDomainDoctorProbeTimeout, defaultDomainDoctorProbe),
		ExpiryWarnDays: envInt(envTrafficCertExpiryDays, defaultTrafficCertDays),
	})
	if !rep.Probed {
		return false, firstNonEmpty(rep.ProbeNote, "the domain does not resolve to this server yet"), nil
	}
	for _, c := range rep.Checks {
		if c.State == domaindoctor.StateFail && c.Tier <= domaindoctor.TierCertificate {
			return false, c.Title + ": " + c.Detail, nil
		}
	}
	return true, "domain doctor: " + rep.Status, nil
}
