package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/appimport"
	"github.com/GLINCKER/levelrail/internal/cutover"
	"github.com/GLINCKER/levelrail/internal/datamigrate"
	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
	"github.com/GLINCKER/levelrail/internal/domaindoctor"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/libdns/libdns"
)

var statusLine = regexp.MustCompile(`HTTP/\d(?:\.\d)? (\d{3})`)

// TestAppImportCutover_Live stages a fixture app on a real Docker daemon,
// drives it with the real application reconciler, and runs the dry run,
// the switch and the rollback through the HTTP API with a fake DNS
// provider. The ingress probe is a real request, made inside the container.
func TestAppImportCutover_Live(t *testing.T) {
	dockertest.SkipIfShort(t)
	client, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	const (
		app    = "cutover-live-fixture"
		image  = "httpd:alpine"
		domain = "live.example.com"
	)
	if _, err := client.InspectByName(ctx, "levelrail-cutover-live-probe"); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}

	cleanup := func() {
		found, _ := client.ListByPrefix(context.Background(), app+"-")
		for _, cs := range found {
			_ = client.Stop(context.Background(), cs.ID, 2*time.Second)
			_ = client.Remove(context.Background(), cs.ID, true)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db)
	cookie := loginTestSession(t, rt, db)
	mgr := &fakeDNSManager{recs: []libdns.Record{rrOf("live", cutoverOldIP)}}
	view := cutoverDNSView{mgr: mgr}
	rt.publicHost = cutoverServerIP
	rt.detectPublicIPs = func(context.Context) []string { return nil }
	rt.lookupHost = func(context.Context, string) ([]string, error) { return []string{cutoverServerIP}, nil }
	rt.dnsResolver = view
	rt.domainAuto.resolveTarget = func(_ context.Context, d string) (*dnsTarget, error) {
		if !strings.HasSuffix(d, "example.com") {
			return nil, dnsrecords.ErrZoneNotFound
		}
		return &dnsTarget{Manager: mgr, Provider: "fake", Zone: "example.com."}, nil
	}
	rt.traffic.doctorProbes = &domaindoctor.Probes{DNS: view, TLS: cutoverEdge{}, HTTP: cutoverEdge{}}
	rt.cutover.cfg = &cutover.Config{HealthTimeout: 60 * time.Second, ProbeTimeout: 20 * time.Second, ProbeInterval: 500 * time.Millisecond,
		VerifyTimeout: 20 * time.Second, VerifyInterval: 500 * time.Millisecond, ResumeMaxAge: time.Hour}
	rt.cutover.httpProbe = func(pctx context.Context, rawURL, _ string) (int, error) {
		u, err := url.Parse(rawURL)
		if err != nil {
			return 0, err
		}
		if u.Scheme == "https" {
			return 0, errors.New("no certificate yet")
		}
		return probeInContainer(pctx, client, app, u.Host, u.Path)
	}

	svc := store.DesiredService{Name: app, Image: image, Port: 0, Suspended: true, Labels: map[string]string{"levelrail.import/session": "live"}}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	rec, _ := json.Marshal(appImportRecord{Entry: appimport.Entry{SourceID: "src-1", Name: app, Kind: appimport.KindApp, Source: appimport.SourceImage, Image: image}})
	sess := store.AppImportSession{ID: "appimp-live", Platform: "coolify", Step: appImportStepCutover, Collision: "suffix", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	item := store.AppImportItem{SessionID: sess.ID, SourceID: "src-1", SourceName: app, TargetName: app, Kind: appimport.KindApp,
		State: store.AppImportVerified, Selected: true, EntryJSON: string(rec), Domains: []string{domain}}
	if err := db.CreateAppImportSession(ctx, sess, []store.AppImportItem{item}); err != nil {
		t.Fatal(err)
	}

	// The real reconciler, as the engine would run it.
	loopCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ctrl := application.New(app, db, client)
		for loopCtx.Err() == nil {
			if res, err := ctrl.Reconcile(loopCtx); err == nil || len(res.Conditions) > 0 {
				_ = db.UpsertConditions(loopCtx, applicationControllerName(app), res.Conditions)
			}
			select {
			case <-loopCtx.Done():
			case <-time.After(500 * time.Millisecond):
			}
		}
	}()
	t.Cleanup(func() { stop(); <-done })

	call := func(method, path, body string) (int, []byte) {
		rec := serve(rt, authedRequest(t, cookie, method, path, body))
		b, _ := io.ReadAll(rec.Body)
		return rec.Code, b
	}
	base := appImportBase + "/sessions/" + sess.ID + "/items/src-1/cutover"
	await := func(id string, states ...string) cutoverRunResource {
		t.Helper()
		var last cutoverRunResource
		waitFor(t, 3*time.Minute, func() bool {
			_, b := call(http.MethodGet, base+"/runs/"+id, "")
			_ = json.Unmarshal(b, &last)
			for _, s := range states {
				if last.State == s {
					return true
				}
			}
			return false
		})
		return last
	}
	running := func() int {
		found, _ := client.ListByPrefix(ctx, app+"-")
		n := 0
		for _, c := range found {
			if c.Running {
				n++
			}
		}
		return n
	}
	ips := func() []string { return view.ips(domain) }

	code, b := call(http.MethodGet, base+"/plan", "")
	var plan cutover.Plan
	if code != http.StatusOK || json.Unmarshal(b, &plan) != nil || plan.Verdict == cutover.VerdictBlocked {
		t.Fatalf("plan = %d %s", code, b)
	}

	// Dry run: starts a real container, probes it, stops it, changes no record.
	code, b = call(http.MethodPost, base+"/runs", `{"mode":"dry_run"}`)
	if code != http.StatusAccepted {
		t.Fatalf("dry run = %d %s", code, b)
	}
	var run cutoverRunResource
	_ = json.Unmarshal(b, &run)
	run = await(run.ID, cutover.StateReady, cutover.StateFailed)
	if run.State != cutover.StateReady {
		t.Fatalf("dry run = %s (%s) steps=%+v", run.State, run.Error, run.Steps)
	}
	if got := ips(); len(got) != 1 || got[0] != cutoverOldIP {
		t.Fatalf("dry run changed DNS: %v", got)
	}
	waitFor(t, time.Minute, func() bool { return running() == 0 })

	// Switch: the container runs for real and DNS now points here.
	code, b = call(http.MethodPost, base+"/runs", `{"mode":"switch","confirm":"`+app+`","accept_warnings":true}`)
	if code != http.StatusAccepted {
		t.Fatalf("switch = %d %s", code, b)
	}
	_ = json.Unmarshal(b, &run)
	run = await(run.ID, cutover.StateLive, cutover.StateFailed, cutover.StateRolledBack)
	if run.State != cutover.StateLive {
		t.Fatalf("switch = %s (%s) steps=%+v", run.State, run.Error, run.Steps)
	}
	if got := ips(); len(got) != 1 || got[0] != cutoverServerIP {
		t.Fatalf("dns after switch = %v", got)
	}
	if n := running(); n != 1 {
		t.Fatalf("running containers = %d, want 1", n)
	}

	// Rollback: record restored, container removed.
	code, b = call(http.MethodPost, base+"/runs/"+run.ID+"/rollback", `{}`)
	if code != http.StatusOK {
		t.Fatalf("rollback = %d %s", code, b)
	}
	if got := ips(); len(got) != 1 || got[0] != cutoverOldIP {
		t.Fatalf("dns after rollback = %v", got)
	}
	waitFor(t, time.Minute, func() bool { return running() == 0 })
}

// probeInContainer sends a GET with a Host header to the app from inside
// its own container, so it works without a route from the host.
func probeInContainer(ctx context.Context, rt docker.Runtime, app, host, path string) (int, error) {
	found, err := rt.ListByPrefix(ctx, app+"-")
	if err != nil {
		return 0, err
	}
	for _, c := range found {
		if !c.Running {
			continue
		}
		out, err := rt.Exec(ctx, c.ID, []string{"sh", "-c", `wget -S -O /dev/null --header "Host: $0" "http://127.0.0.1$1" 2>&1`, host, path})
		if err != nil {
			return 0, err
		}
		b, _ := io.ReadAll(out)
		_ = out.Close()
		if m := statusLine.FindSubmatch(b); m != nil {
			var n int
			for _, ch := range m[1] {
				n = n*10 + int(ch-'0')
			}
			return n, nil
		}
		return 0, errors.New("no HTTP status in the probe output: " + strings.TrimSpace(string(b)))
	}
	return 0, errors.New("the app has no running container")
}

var _ datamigrate.Resolver = cutoverDNSView{}
