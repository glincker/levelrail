package api

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/datamigrate"
	"github.com/GLINCKER/levelrail/internal/migrationgate"
	"github.com/GLINCKER/levelrail/internal/platformimport/fakecoolify"
	"github.com/GLINCKER/levelrail/internal/store"
)

func (h *appImportHarness) decode(rec *httptest.ResponseRecorder, into any) {
	h.t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
		h.t.Fatalf("decode: %v: %s", err, rec.Body.String())
	}
}

func (h *appImportHarness) newSession() (string, appImportSessionResource) {
	h.t.Helper()
	rec := h.call(http.MethodPost, appImportBase+"/sessions", h.connectBody(""))
	if rec.Code != http.StatusCreated {
		h.t.Fatalf("create session: %d %s", rec.Code, rec.Body.String())
	}
	v := h.view(rec)
	return v.ID, v
}

func (h *appImportHarness) selectOnly(id string, sourceIDs ...string) {
	h.t.Helper()
	raw, _ := json.Marshal(map[string]any{"selected": sourceIDs})
	if rec := h.call(http.MethodPut, appImportBase+"/sessions/"+id+"/plan", string(raw)); rec.Code != http.StatusOK {
		h.t.Fatalf("select: %d %s", rec.Code, rec.Body.String())
	}
}

func (h *appImportHarness) progress(id string) migrationJobProgress {
	h.t.Helper()
	rec := h.call(http.MethodGet, appImportBase+"/sessions/"+id+"/progress", "")
	if rec.Code != http.StatusOK {
		h.t.Fatalf("progress: %d %s", rec.Code, rec.Body.String())
	}
	var p migrationJobProgress
	h.decode(rec, &p)
	return p
}

func (p migrationJobProgress) item(name string) migrationJobItem {
	for _, it := range p.Items {
		if it.Name == name {
			return it
		}
	}
	return migrationJobItem{}
}

func (p migrationJobProgress) wants(code string) bool {
	for _, n := range p.Next {
		if n == code {
			return true
		}
	}
	return false
}

func TestMigrationConnectionTest(t *testing.T) {
	h := newAppImportHarness(t)

	rec := h.call(http.MethodPost, appImportBase+"/test", h.connectBody(""))
	if rec.Code != http.StatusOK {
		t.Fatalf("test: %d %s", rec.Code, rec.Body.String())
	}
	h.noLeaks(rec.Body.String())
	var ok migrationTestResult
	h.decode(rec, &ok)
	if !ok.OK || ok.Apps == 0 || ok.Platform != "coolify" {
		t.Fatalf("a reachable source should report what it found: %+v", ok)
	}
	if sessions, _ := h.db.ListAppImportSessions(context.Background()); len(sessions) != 0 {
		t.Fatalf("a connection test must store nothing, found %d session(s)", len(sessions))
	}

	bad := strings.Replace(h.connectBody(""), fakecoolify.Token, "wrong-fake-token", 1)
	rec = h.call(http.MethodPost, appImportBase+"/test", bad)
	var denied migrationTestResult
	h.decode(rec, &denied)
	if rec.Code != http.StatusOK || denied.OK || denied.Failure != testCredentials {
		t.Fatalf("a rejected token should classify as credentials: %d %+v", rec.Code, denied)
	}
	if strings.Contains(rec.Body.String(), "wrong-fake-token") {
		t.Fatalf("the submitted token was echoed back: %s", rec.Body.String())
	}

	blocked := strings.Replace(h.connectBody(""), `,"allow_loopback":true`, "", 1)
	blocked = strings.Replace(blocked, h.src.URL, "http://169.254.169.254", 1)
	rec = h.call(http.MethodPost, appImportBase+"/test", blocked)
	var res migrationTestResult
	h.decode(rec, &res)
	if res.OK || res.Failure != testBlocked {
		t.Fatalf("a metadata address must be refused as blocked: %+v", res)
	}
}

func TestMigrationTestNeverReachesLogsOrAudit(t *testing.T) {
	h := newAppImportHarness(t)
	var logs bytes.Buffer
	h.rt.logger = slog.New(slog.NewTextHandler(&logs, nil))
	h.call(http.MethodPost, appImportBase+"/test", h.connectBody(""))
	bad := strings.Replace(h.connectBody(""), fakecoolify.Token, "wrong-fake-token", 1)
	h.call(http.MethodPost, appImportBase+"/test", bad)
	id, _ := h.newSession()
	h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/connect", h.connectBody(""))

	for _, secret := range []string{fakecoolify.Token, "wrong-fake-token"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("a source credential reached the logs: %s", logs.String())
		}
		events, err := h.db.ListAuditEntries(context.Background(), 500, nil, store.AuditEntryFilter{})
		if err != nil || len(events) == 0 {
			t.Fatalf("expected audit entries for the migration calls: %v", err)
		}
		raw, _ := json.Marshal(events)
		if strings.Contains(string(raw), secret) {
			t.Fatalf("a source credential reached the audit log: %s", raw)
		}
	}
}

func TestMigrationProgressNextActions(t *testing.T) {
	h := newAppImportHarness(t)
	id, v := h.newSession()
	h.selectOnly(id, v.item("searxng").SourceID)

	p := h.progress(id)
	if p.Total != 1 || p.Phase != "attention" || !p.wants(nextStage) || p.wants(nextReconnect) {
		t.Fatalf("a fresh connected job should ask for staging: %+v", p)
	}
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/stage", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("stage: %d %s", rec.Code, rec.Body.String())
	}
	p = h.progress(id)
	if p.item("searxng").State != store.AppImportStaged || !p.wants(nextVerify) || p.wants(nextStage) {
		t.Fatalf("after staging the job should ask for verification: %+v", p)
	}
	h.noLeaks(h.call(http.MethodGet, appImportBase+"/sessions/"+id+"/progress", "").Body.String())
}

func TestStageRetriesOnlyTheRequestedItem(t *testing.T) {
	h := newAppImportHarness(t)
	id, v := h.newSession()
	searx, web := v.item("searxng").SourceID, v.item("memgraph").SourceID
	h.selectOnly(id, searx, web)

	raw, _ := json.Marshal(map[string]any{"items": []string{searx}})
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/stage", string(raw)); rec.Code != http.StatusOK {
		t.Fatalf("stage one: %d %s", rec.Code, rec.Body.String())
	}
	p := h.progress(id)
	if p.item("searxng").State != store.AppImportStaged || p.item("memgraph").State != store.AppImportPlanned {
		t.Fatalf("only the requested item may be staged: %+v", p.Items)
	}

	items, _ := h.db.ListAppImportItems(context.Background(), id)
	for _, it := range items {
		if it.SourceID == searx {
			it.State, it.Reason = store.AppImportStageFailed, "boom"
			if err := h.db.SaveAppImportItem(context.Background(), it, time.Now()); err != nil {
				t.Fatal(err)
			}
		}
	}
	p = h.progress(id)
	if got := p.item("searxng"); !got.Retryable || p.Failed != 1 || !p.wants(nextRetry) {
		t.Fatalf("a failed item must be retryable and surfaced: %+v", p)
	}
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/stage", string(raw)); rec.Code != http.StatusOK {
		t.Fatalf("retry: %d %s", rec.Code, rec.Body.String())
	}
	p = h.progress(id)
	if p.item("searxng").State == store.AppImportStageFailed || p.item("memgraph").State != store.AppImportPlanned {
		t.Fatalf("retry should fix only the failed item: %+v", p.Items)
	}
}

// restart builds a second control plane over the same database, which is what a crash and
// reboot looks like: stored state survives, the in-memory source credential does not.
func (h *appImportHarness) restart() {
	h.t.Helper()
	old := h.rt
	h.rt = NewRouter(nil, testBrand(), h.db, WithSecretSetter(h.setter), WithBuilder(&fakeBuilder{tag: "img:sha"}), WithGitSourceSecrets(newFakeGitSourceSecrets()))
	h.rt.gitSourceFetch = old.gitSourceFetch
}

func TestJobResumesAfterTheControlPlaneRestarts(t *testing.T) {
	h := newAppImportHarness(t)
	id, v := h.newSession()
	searx, mem := v.item("searxng").SourceID, v.item("memgraph").SourceID
	h.selectOnly(id, searx, mem)
	raw, _ := json.Marshal(map[string]any{"items": []string{searx}})
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/stage", string(raw)); rec.Code != http.StatusOK {
		t.Fatalf("first half: %d %s", rec.Code, rec.Body.String())
	}
	before := h.progress(id).item("searxng").Target

	h.restart()
	p := h.progress(id)
	if p.Connected || !p.wants(nextReconnect) || p.item("searxng").State != store.AppImportStaged || p.item("memgraph").State != store.AppImportPlanned {
		t.Fatalf("after a restart the job keeps its progress but asks for the credential again: %+v", p)
	}
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/stage", `{}`); rec.Code != http.StatusConflict {
		t.Fatalf("staging without the credential should be refused, got %d", rec.Code)
	}
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/connect", h.connectBody("")); rec.Code != http.StatusOK {
		t.Fatalf("reconnect: %d %s", rec.Code, rec.Body.String())
	}
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/stage", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("second half: %d %s", rec.Code, rec.Body.String())
	}
	p = h.progress(id)
	if p.item("searxng").State != store.AppImportStaged || p.item("memgraph").State == store.AppImportPlanned {
		t.Fatalf("resume should finish the rest: %+v", p.Items)
	}
	if after := p.item("searxng").Target; after != before || before == "" {
		t.Fatalf("resume must not recreate an app that was already staged: %q then %q", before, after)
	}
}

func TestVerifyInterruptedByRestartCanBeRetried(t *testing.T) {
	h := newAppImportHarness(t)
	id, v := h.newSession()
	searx := v.item("searxng").SourceID
	h.selectOnly(id, searx)
	h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/stage", `{}`)
	items, _ := h.db.ListAppImportItems(context.Background(), id)
	for _, it := range items {
		if it.SourceID == searx {
			it.State = store.AppImportBuilding
			_ = h.db.SaveAppImportItem(context.Background(), it, time.Now())
		}
	}
	p := h.progress(id)
	if got := p.item("searxng"); got.State != store.AppImportVerifyFailed || !got.Retryable {
		t.Fatalf("a build orphaned by a crash must show as retryable, not building forever: %+v", got)
	}
	h.ready("searxng")
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/verify", `{}`); rec.Code != http.StatusAccepted {
		t.Fatalf("verify after crash: %d %s", rec.Code, rec.Body.String())
	}
	waitFor(t, 10*time.Second, func() bool { return h.progress(id).item("searxng").State == store.AppImportVerified })
}

type syncRecorder struct {
	mu   sync.Mutex
	rec  *httptest.ResponseRecorder
	done chan struct{}
}

func (s *syncRecorder) Header() http.Header { return s.rec.Header() }
func (s *syncRecorder) WriteHeader(c int)   { s.rec.WriteHeader(c) }
func (s *syncRecorder) Flush()              {}
func (s *syncRecorder) Write(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rec.Write(b)
}
func (s *syncRecorder) body() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rec.Body.String()
}

func TestEventsStreamShowsProgressAndHonoursLastEventID(t *testing.T) {
	t.Setenv(envMigrateEventsInterval, "20ms")
	h := newAppImportHarness(t)
	id, v := h.newSession()
	h.selectOnly(id, v.item("searxng").SourceID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &syncRecorder{rec: httptest.NewRecorder(), done: make(chan struct{})}
	go func() {
		defer close(w.done)
		req := authedRequest(t, h.cookie, http.MethodGet, appImportBase+"/sessions/"+id+"/events", "").WithContext(ctx)
		h.rt.Handler().ServeHTTP(w, req)
	}()
	waitFor(t, 5*time.Second, func() bool { return strings.Contains(w.body(), "data: ") })
	if !strings.Contains(w.body(), `"planned":1`) {
		t.Fatalf("first event should carry the planned item: %s", w.body())
	}
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/stage", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("stage: %d", rec.Code)
	}
	waitFor(t, 5*time.Second, func() bool { return strings.Contains(w.body(), `"staged":1`) })
	cancel()
	<-w.done
	if strings.Contains(w.body(), fakecoolify.Token) {
		t.Fatal("the event stream leaked a credential")
	}

	rev := h.progress(id).Revision
	ctx2, cancel2 := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel2()
	w2 := &syncRecorder{rec: httptest.NewRecorder(), done: make(chan struct{})}
	req := authedRequest(t, h.cookie, http.MethodGet, appImportBase+"/sessions/"+id+"/events", "").WithContext(ctx2)
	req.Header.Set("Last-Event-ID", rev)
	h.rt.Handler().ServeHTTP(w2, req)
	if strings.Contains(w2.body(), "data: ") {
		t.Fatalf("a reconnecting client that is up to date should not be sent the same state again: %s", w2.body())
	}
}

func selfSignedFor(t *testing.T, domain string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: domain}, DNSNames: []string{domain},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool
}

type guideDNSView struct{ addrs []string }

func (s guideDNSView) Lookup(context.Context, string) datamigrate.DomainState {
	return datamigrate.DomainState{Addrs: s.addrs, TTL: 60}
}

// fakeEdge stands in for this node: a TLS server for domain that answers the health path.
func fakeEdge(t *testing.T, domain string, status int) *datamigrate.Prober {
	t.Helper()
	cert, pool := selfSignedFor(t, domain)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return &datamigrate.Prober{
		Resolver: guideDNSView{addrs: []string{cutoverServerIP}},
		RootCAs:  pool,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
		},
		Timeout: 3 * time.Second,
	}
}

func (h *appImportHarness) verifiedSearxng() (id string, domain string) {
	h.t.Helper()
	id, v := h.newSession()
	it := v.item("searxng")
	h.selectOnly(id, it.SourceID)
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/stage", `{}`); rec.Code != http.StatusOK {
		h.t.Fatalf("stage: %d %s", rec.Code, rec.Body.String())
	}
	h.ready("searxng")
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/verify", `{}`); rec.Code != http.StatusAccepted {
		h.t.Fatalf("verify: %d %s", rec.Code, rec.Body.String())
	}
	waitFor(h.t, 10*time.Second, func() bool { return h.progress(id).item("searxng").State == store.AppImportVerified })
	if len(it.Domains) == 0 {
		h.t.Fatal("fixture app has no domain")
	}
	return id, it.Domains[0]
}

func (h *appImportHarness) retire(id, body string) migrationgate.Report {
	h.t.Helper()
	method := http.MethodPost
	if body == "" {
		method = http.MethodGet
	}
	rec := h.call(method, appImportBase+"/sessions/"+id+"/retire?target_ip="+cutoverServerIP, body)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("retire: %d %s", rec.Code, rec.Body.String())
	}
	var r migrationgate.Report
	h.decode(rec, &r)
	return r
}

func gateOf(r migrationgate.Report, id string) migrationgate.Gate {
	for _, g := range r.Gates {
		if g.ID == id {
			return g
		}
	}
	return migrationgate.Gate{}
}

func TestProbeOnlyTouchesDomainsOfTheJob(t *testing.T) {
	h := newAppImportHarness(t)
	id, domain := h.verifiedSearxng()
	h.rt.migrationProber = fakeEdge(t, domain, http.StatusOK)

	rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/probe?target_ip="+cutoverServerIP, `{"domain":"internal.corp.example"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("a foreign domain must be refused, got %d %s", rec.Code, rec.Body.String())
	}
	rec = h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/probe?target_ip="+cutoverServerIP, `{"domain":"`+domain+`","kind":"bogus"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("an unknown kind must be a 400, got %d", rec.Code)
	}
	rec = h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/probe?target_ip="+cutoverServerIP, `{"domain":"`+strings.ToUpper(domain)+`"}`)
	var res migrationProbeResult
	h.decode(rec, &res)
	if rec.Code != http.StatusOK || !res.OK || res.DNS == nil || len(res.HTTP) != 2 {
		t.Fatalf("a healthy domain should pass both probes: %d %+v", rec.Code, res)
	}
}

func TestProbeReportsAWrongDNSAndADeadApp(t *testing.T) {
	h := newAppImportHarness(t)
	id, domain := h.verifiedSearxng()
	p := fakeEdge(t, domain, http.StatusBadGateway)
	p.Resolver = guideDNSView{addrs: []string{"203.0.113.9"}}
	h.rt.migrationProber = p

	rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/probe?target_ip="+cutoverServerIP, `{"domain":"`+domain+`","kind":"dns"}`)
	var res migrationProbeResult
	h.decode(rec, &res)
	if res.OK || res.DNS == nil || res.DNS.Status != datamigrate.CheckFail || res.HTTP != nil {
		t.Fatalf("old DNS must fail the dns probe only: %+v", res)
	}
	rec = h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/probe?target_ip="+cutoverServerIP, `{"domain":"`+domain+`","kind":"http"}`)
	var httpRes migrationProbeResult
	h.decode(rec, &httpRes)
	if httpRes.OK || httpRes.DNS != nil || checksPass(httpRes.HTTP...) {
		t.Fatalf("a 502 must fail the response probe: %+v", httpRes)
	}
}

func TestSafeToDeleteTurnsGreenOnlyWithEvidence(t *testing.T) {
	h := newAppImportHarness(t)
	id, domain := h.verifiedSearxng()
	h.rt.migrationProber = fakeEdge(t, domain, http.StatusOK)
	acks := `"acknowledged":["source-backup","traffic-cycle"]`

	r := h.retire(id, "")
	if r.Safe || gateOf(r, migrationgate.GateRouted).Status != migrationgate.Fail || gateOf(r, migrationgate.GateDNS).Status != migrationgate.Pending {
		t.Fatalf("a staged-only job is not safe, and a read must not run probes: %+v", r.Gates)
	}
	if len(r.Fallback) == 0 {
		t.Fatal("rollback guidance missing")
	}

	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/items/"+h.progress(id).item("searxng").SourceID+"/route", `{"enable":true,"ignore_volumes":true}`); rec.Code != http.StatusOK {
		t.Fatalf("route: %d %s", rec.Code, rec.Body.String())
	}
	r = h.retire(id, `{"probe":true,`+acks+`}`)
	if gateOf(r, migrationgate.GateRouted).Status != migrationgate.Pass || gateOf(r, migrationgate.GateDNS).Status != migrationgate.Pass || gateOf(r, migrationgate.GateResponds).Status != migrationgate.Pass {
		t.Fatalf("routed, resolving and answering should pass: %+v", r.Gates)
	}
	if r.Safe || gateOf(r, migrationgate.GateLeftovers).Status != migrationgate.Manual {
		t.Fatalf("resources the importer could not move must hold the verdict until confirmed: %+v", r.Gates)
	}
	r = h.retire(id, `{"probe":true,"acknowledged":["source-backup","traffic-cycle","leftovers"]}`)
	if !r.Safe {
		t.Fatalf("every gate has evidence, the verdict should be safe: %+v", r.Gates)
	}
}

func TestSafeToDeleteStaysRedWhenTheAppStopsAnswering(t *testing.T) {
	h := newAppImportHarness(t)
	id, domain := h.verifiedSearxng()
	h.rt.migrationProber = fakeEdge(t, domain, http.StatusInternalServerError)
	r := h.retire(id, `{"probe":true,"acknowledged":["source-backup","traffic-cycle"]}`)
	if r.Safe || gateOf(r, migrationgate.GateResponds).Status != migrationgate.Fail {
		t.Fatalf("a failing app must keep the old instance protected: %+v", r.Gates)
	}
}

func TestMigrationGuideRoutesRequireAuth(t *testing.T) {
	h := newAppImportHarness(t)
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, appImportBase + "/test"},
		{http.MethodGet, appImportBase + "/sessions/x/progress"},
		{http.MethodGet, appImportBase + "/sessions/x/events"},
		{http.MethodPost, appImportBase + "/sessions/x/probe"},
		{http.MethodGet, appImportBase + "/sessions/x/retire"},
		{http.MethodPost, appImportBase + "/sessions/x/retire"},
	} {
		rec := httptest.NewRecorder()
		h.rt.Handler().ServeHTTP(rec, httptest.NewRequest(c.method, c.path, strings.NewReader("{}")))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a session = %d, want 401", c.method, c.path, rec.Code)
		}
	}
}
