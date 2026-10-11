package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cutover"
	"github.com/GLINCKER/levelrail/internal/datamigrate"
	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/domaindoctor"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/libdns/libdns"
)

const (
	cutoverServerIP = "198.51.100.4"
	cutoverOldIP    = "203.0.113.9"
)

// cutoverDNSView answers resolver and doctor lookups from the fake zone, so
// the world changes when the runner changes the record.
type cutoverDNSView struct{ mgr *fakeDNSManager }

func (v cutoverDNSView) ips(domain string) []string {
	name := strings.TrimSuffix(domain, ".example.com")
	var out []string
	recs, _ := v.mgr.GetRecords(context.Background(), "example.com.")
	for _, r := range recs {
		if rr := r.RR(); rr.Name == name && rr.Type == "A" {
			out = append(out, rr.Data)
		}
	}
	return out
}

func (v cutoverDNSView) Lookup(_ context.Context, domain string) datamigrate.DomainState {
	return datamigrate.DomainState{Addrs: v.ips(domain), TTL: 60, Authoritative: true}
}

func (v cutoverDNSView) Answers(_ context.Context, domain string) []domaindoctor.ResolverAnswer {
	return []domaindoctor.ResolverAnswer{{Resolver: "1.1.1.1", IPv4: v.ips(domain)}}
}
func (cutoverDNSView) CNAME(context.Context, string) (string, error) { return "", nil }
func (cutoverDNSView) CAA(context.Context, string) ([]domaindoctor.CAARecord, error) {
	return nil, nil
}

type cutoverEdge struct{}

func (cutoverEdge) Handshake(_ context.Context, _, sni string) (domaindoctor.PeerCert, error) {
	return domaindoctor.PeerCert{Subject: sni, Issuer: "Fake CA", DNSNames: []string{sni}, NotAfter: time.Now().Add(60 * 24 * time.Hour), Verified: true}, nil
}

func (cutoverEdge) Get(context.Context, string, string, string, string) (domaindoctor.HTTPResult, error) {
	return domaindoctor.HTTPResult{Status: http.StatusOK}, nil
}

type cutoverFixture struct {
	*appImportHarness
	mgr     *fakeDNSManager
	session string
	item    string
	base    string
}

func (f *cutoverFixture) setup() {
	f.t.Helper()
	ctx := context.Background()
	rec := f.call(http.MethodPost, appImportBase+"/sessions", f.connectBody(""))
	if rec.Code != http.StatusCreated {
		f.t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	v := f.view(rec)
	f.session, f.item = v.ID, v.item("searxng").SourceID
	rec = f.call(http.MethodPut, appImportBase+"/sessions/"+f.session+"/plan", `{"selected":["`+f.item+`"]}`)
	if rec.Code != http.StatusOK {
		f.t.Fatalf("plan: %d %s", rec.Code, rec.Body.String())
	}
	if rec = f.call(http.MethodPost, appImportBase+"/sessions/"+f.session+"/stage", `{}`); rec.Code != http.StatusOK {
		f.t.Fatalf("stage: %d %s", rec.Code, rec.Body.String())
	}
	f.ready("searxng")
	if rec = f.call(http.MethodPost, appImportBase+"/sessions/"+f.session+"/verify", `{}`); rec.Code != http.StatusAccepted {
		f.t.Fatalf("verify: %d %s", rec.Code, rec.Body.String())
	}
	waitFor(f.t, 10*time.Second, func() bool {
		items, _ := f.db.ListAppImportItems(ctx, f.session)
		for _, it := range items {
			if it.SourceID == f.item {
				return it.State == store.AppImportVerified
			}
		}
		return false
	})
	f.base = appImportBase + "/sessions/" + f.session + "/items/" + f.item + "/cutover"
}

func newCutoverFixture(t *testing.T) *cutoverFixture {
	t.Helper()
	h := newAppImportHarness(t)
	mgr := &fakeDNSManager{recs: []libdns.Record{rrOf("search", cutoverOldIP)}}
	view := cutoverDNSView{mgr: mgr}
	rt := h.rt
	rt.publicHost = cutoverServerIP
	rt.detectPublicIPs = func(context.Context) []string { return nil }
	rt.lookupHost = func(context.Context, string) ([]string, error) { return []string{cutoverServerIP}, nil }
	rt.dnsResolver = view
	rt.domainAuto.resolveTarget = func(_ context.Context, domain string) (*dnsTarget, error) {
		if !strings.HasSuffix(domain, "example.com") {
			return nil, dnsrecords.ErrZoneNotFound
		}
		return &dnsTarget{Manager: mgr, Provider: "fake", Zone: "example.com."}, nil
	}
	rt.cutover.httpProbe = func(context.Context, string, string) (int, error) { return http.StatusOK, nil }
	rt.cutover.cfg = &cutover.Config{HealthTimeout: 5 * time.Second, ProbeTimeout: 2 * time.Second, ProbeInterval: 10 * time.Millisecond,
		VerifyTimeout: 3 * time.Second, VerifyInterval: 20 * time.Millisecond, ResumeMaxAge: time.Hour}
	rt.traffic.doctorProbes = &domaindoctor.Probes{DNS: view, TLS: cutoverEdge{}, HTTP: cutoverEdge{}}
	f := &cutoverFixture{appImportHarness: h, mgr: mgr}
	f.setup()
	return f
}

func (f *cutoverFixture) decodeRun(rec interface{ Bytes() []byte }) cutoverRunResource {
	f.t.Helper()
	var r cutoverRunResource
	if err := json.Unmarshal(rec.Bytes(), &r); err != nil {
		f.t.Fatalf("decode run: %v", err)
	}
	return r
}

func (f *cutoverFixture) start(body string) (cutoverRunResource, int) {
	f.t.Helper()
	rec := f.call(http.MethodPost, f.base+"/runs", body)
	if rec.Code != http.StatusAccepted {
		return cutoverRunResource{}, rec.Code
	}
	return f.decodeRun(rec.Body), rec.Code
}

func (f *cutoverFixture) await(id string, states ...string) cutoverRunResource {
	f.t.Helper()
	var last cutoverRunResource
	waitFor(f.t, 15*time.Second, func() bool {
		rec := f.call(http.MethodGet, f.base+"/runs/"+id, "")
		last = f.decodeRun(rec.Body)
		for _, s := range states {
			if last.State == s {
				return true
			}
		}
		return false
	})
	return last
}

func (f *cutoverFixture) currentIPs() []string {
	return cutoverDNSView{mgr: f.mgr}.ips("search.example.com")
}

func TestAppImportCutoverDryRunSwitchAndRollback(t *testing.T) {
	f := newCutoverFixture(t)
	ctx := context.Background()
	sourceRequests := len(f.fake.Requests())

	rec := f.call(http.MethodGet, f.base+"/plan", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("plan: %d %s", rec.Code, rec.Body.String())
	}
	var plan cutover.Plan
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Verdict == cutover.VerdictBlocked || len(plan.Domains) != 1 || plan.Domains[0].Method != cutover.MethodDNS {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Domains[0].Desired == nil || plan.Domains[0].Desired.Value != cutoverServerIP || len(plan.Domains[0].Replace) != 1 {
		t.Fatalf("domain plan = %+v", plan.Domains[0])
	}

	// Dry run: everything except the switch.
	run, code := f.start(`{"mode":"dry_run"}`)
	if code != http.StatusAccepted {
		t.Fatalf("dry run start = %d", code)
	}
	run = f.await(run.ID, cutover.StateReady, cutover.StateFailed)
	if run.State != cutover.StateReady {
		t.Fatalf("dry run = %s (%s)", run.State, run.Error)
	}
	if ips := f.currentIPs(); len(ips) != 1 || ips[0] != cutoverOldIP {
		t.Fatalf("a dry run changed DNS: %v", ips)
	}
	if svc, _ := f.db.GetDesiredService(ctx, "searxng"); !svc.Suspended || len(svc.Domains) != 0 {
		t.Fatalf("a dry run left the app serving: suspended=%v domains=%v", svc.Suspended, svc.Domains)
	}

	// The switch needs the typed app name.
	if _, code := f.start(`{"mode":"switch","accept_warnings":true}`); code != http.StatusBadRequest {
		t.Fatalf("switch without confirmation = %d", code)
	}
	run, code = f.start(`{"mode":"switch","confirm":"searxng","accept_warnings":true}`)
	if code != http.StatusAccepted {
		t.Fatalf("switch start = %d", code)
	}
	run = f.await(run.ID, cutover.StateLive, cutover.StateFailed, cutover.StateRolledBack)
	if run.State != cutover.StateLive {
		t.Fatalf("switch = %s (%s)", run.State, run.Error)
	}
	if ips := f.currentIPs(); len(ips) != 1 || ips[0] != cutoverServerIP {
		t.Fatalf("dns after switch = %v", ips)
	}
	if len(run.Domains) != 1 || len(run.Domains[0].Previous) != 1 || run.Domains[0].Previous[0].Value != cutoverOldIP || !run.Rollbackable {
		t.Fatalf("undo data = %+v", run.Domains)
	}
	if svc, _ := f.db.GetDesiredService(ctx, "searxng"); svc.Suspended || len(svc.Domains) != 1 {
		t.Fatalf("live app: suspended=%v domains=%v", svc.Suspended, svc.Domains)
	}
	if _, code := f.start(`{"mode":"switch","confirm":"searxng","accept_warnings":true}`); code != http.StatusConflict {
		t.Fatalf("second switch while live = %d, want 409", code)
	}

	// One-click rollback restores the record and stops the app.
	rec = f.call(http.MethodPost, f.base+"/runs/"+run.ID+"/rollback", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("rollback = %d %s", rec.Code, rec.Body.String())
	}
	back := f.decodeRun(rec.Body)
	if back.State != cutover.StateRolledBack {
		t.Fatalf("rollback state = %s (%s)", back.State, back.Error)
	}
	if ips := f.currentIPs(); len(ips) != 1 || ips[0] != cutoverOldIP {
		t.Fatalf("dns after rollback = %v", ips)
	}
	if svc, _ := f.db.GetDesiredService(ctx, "searxng"); !svc.Suspended || len(svc.Domains) != 0 {
		t.Fatalf("rolled back app: suspended=%v domains=%v", svc.Suspended, svc.Domains)
	}
	items, _ := f.db.ListAppImportItems(ctx, f.session)
	for _, it := range items {
		if it.SourceID == f.item && it.State != store.AppImportVerified {
			t.Fatalf("import item state after rollback = %s", it.State)
		}
	}

	// The source platform was never written to or even asked again.
	if v := f.fake.Violations(); len(v) != 0 {
		t.Fatalf("the source received non-GET requests: %v", v)
	}
	if got := len(f.fake.Requests()); got != sourceRequests {
		t.Fatalf("the source received %d request(s) during cutover", got-sourceRequests)
	}
}

func TestAppImportCutoverAutoRollbackWhenVerificationFails(t *testing.T) {
	f := newCutoverFixture(t)
	f.rt.traffic.doctorProbes = &domaindoctor.Probes{DNS: cutoverDNSView{mgr: &fakeDNSManager{}}, TLS: cutoverEdge{}, HTTP: cutoverEdge{}}
	run, code := f.start(`{"mode":"switch","confirm":"searxng","accept_warnings":true}`)
	if code != http.StatusAccepted {
		t.Fatalf("start = %d", code)
	}
	run = f.await(run.ID, cutover.StateLive, cutover.StateFailed, cutover.StateRolledBack)
	if run.State != cutover.StateRolledBack || run.Error == "" {
		t.Fatalf("state = %s (%s)", run.State, run.Error)
	}
	if ips := f.currentIPs(); len(ips) != 1 || ips[0] != cutoverOldIP {
		t.Fatalf("dns was not restored: %v", ips)
	}
}

func TestAppImportCutoverNeedsAVerifiedApp(t *testing.T) {
	f := newCutoverFixture(t)
	ctx := context.Background()
	items, _ := f.db.ListAppImportItems(ctx, f.session)
	for _, it := range items {
		if it.SourceID == f.item {
			it.State = store.AppImportStaged
			if err := f.db.SaveAppImportItem(ctx, it, time.Now()); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, code := f.start(`{"mode":"dry_run"}`); code != http.StatusConflict {
		t.Fatalf("dry run of an unverified app = %d", code)
	}
}
