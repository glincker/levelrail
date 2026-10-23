package trafficexport

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
)

type fakeSource struct {
	mu    sync.Mutex
	specs []SinkSpec
}

func (f *fakeSource) LoadSpecs(context.Context) ([]SinkSpec, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SinkSpec(nil), f.specs...), nil
}

func (f *fakeSource) set(specs ...SinkSpec) {
	f.mu.Lock()
	f.specs = specs
	f.mu.Unlock()
}

// receiver is a webhook endpoint that records bodies and answers with a settable status.
type receiver struct {
	srv    *httptest.Server
	mu     sync.Mutex
	bodies []string
	status int
	gate   chan struct{}
}

func newReceiver(t *testing.T, status int) *receiver {
	t.Helper()
	r := &receiver{status: status}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if r.gate != nil {
			select {
			case <-r.gate:
			case <-req.Context().Done():
				return
			}
		}
		b, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.bodies = append(r.bodies, string(b))
		code := r.status
		r.mu.Unlock()
		w.WriteHeader(code)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *receiver) all() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.bodies, "\n")
}

func (r *receiver) setStatus(code int) {
	r.mu.Lock()
	r.status = code
	r.mu.Unlock()
}

func urlSecret(u string) map[string]string {
	return map[string]string{SecretURL: u} //nolint:gosec // fixture URL, not a credential
}

func webhookSpec(id string, r *receiver, fields []string, trunc bool) SinkSpec {
	return SinkSpec{
		ID: id, Name: id, Kind: SinkWebhook, Enabled: true, Fields: fields, IPTruncate: trunc, Sample: 1,
		Secrets: map[string]string{SecretURL: r.srv.URL + "/in/SECRETPATH"},
	}
}

func testService(t *testing.T, specs ...SinkSpec) (*Service, *fakeSource) {
	t.Helper()
	src := &fakeSource{specs: specs}
	opts := Options{QueueSize: 100, BatchSize: 1, FlushInterval: 5 * time.Millisecond, MaxAttempts: 2, BackoffBase: time.Millisecond, BackoffMax: 2 * time.Millisecond}
	svc := NewService(src, HTTPOptions{AllowPrivate: true, Timeout: 2 * time.Second, Namespace: "acme"}, opts,
		func(key string) string { return "app-" + key },
		func(context.Context) ([]byte, error) { return []byte("test-salt"), nil }, nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		svc.Close(ctx)
	})
	if err := svc.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	return svc, src
}

func identityRecord() ingress.AccessRecord {
	return ingress.AccessRecord{
		Time: time.Now(), Host: "example.test", Method: "GET", Path: "/home", Status: 200, Millis: 4, RequestID: "req-1",
		Identity: &ingress.RequestIdentity{ClientIP: "203.0.113.77", UserAgent: "Mozilla/5.0 SecretAgent", ReferrerHost: "news.example"},
	}
}

func TestServiceNoIdentityLeaksUnlessOptedIn(t *testing.T) {
	plain, optIn := newReceiver(t, 200), newReceiver(t, 200)
	svc, _ := testService(t,
		webhookSpec("plain", plain, nil, true),
		webhookSpec("optin", optIn, []string{FieldClientIP, FieldUserAgent, FieldVisitorID, FieldReferrerHost}, true),
	)
	svc.Tap("k", identityRecord())
	waitFor(t, func() bool { return plain.all() != "" && optIn.all() != "" })

	for _, leak := range []string{"203.0.113", "SecretAgent", "news.example", "client_ip", "user_agent", "visitor_id", "referrer_host"} {
		if strings.Contains(plain.all(), leak) {
			t.Errorf("sink without opt-in received %q: %s", leak, plain.all())
		}
	}
	var ev Event
	if err := json.Unmarshal([]byte(strings.TrimSpace(optIn.all())), &ev); err != nil {
		t.Fatalf("decode opt-in body: %v", err)
	}
	if ev.ClientIP != "203.0.113.0" || ev.UserAgent != "Mozilla/5.0 SecretAgent" || ev.ReferrerHost != "news.example" || len(ev.VisitorID) != visitorIDHexLen {
		t.Errorf("opt-in event = %+v, want truncated ip, ua, referrer and a visitor id", ev)
	}
	if ev.App != "app-k" || ev.Path != "/home" {
		t.Errorf("event = %+v, want app resolved from the route key", ev)
	}
}

func TestServiceNoIdentityWantedWithoutOptIn(t *testing.T) {
	r := newReceiver(t, 200)
	svc, src := testService(t, webhookSpec("plain", r, nil, true))
	if svc.IdentityWanted() {
		t.Fatal("identity capture must stay off while no sink opted in")
	}
	src.set(webhookSpec("plain", r, []string{FieldUserAgent}, true))
	if err := svc.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !svc.IdentityWanted() {
		t.Error("identity capture should turn on once a sink opts in")
	}
	src.set(webhookSpec("plain", r, []string{FieldReferrerHost}, false))
	_ = svc.Reload(context.Background())
	if !svc.IdentityWanted() {
		t.Error("referrer opt-in also needs the capture")
	}
	src.set()
	_ = svc.Reload(context.Background())
	if svc.IdentityWanted() {
		t.Error("capture should turn off when the opted-in sink is removed")
	}
}

func TestServiceQueueFullDropsAndCountsWithoutBlocking(t *testing.T) {
	r := newReceiver(t, 200)
	r.gate = make(chan struct{})
	t.Cleanup(func() { close(r.gate) })
	src := &fakeSource{specs: []SinkSpec{webhookSpec("slow", r, nil, true)}}
	svc := NewService(src, HTTPOptions{AllowPrivate: true, Timeout: 5 * time.Second}, Options{QueueSize: 2, BatchSize: 1, FlushInterval: time.Hour, MaxAttempts: 1}, nil, nil, nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		svc.Close(ctx)
	})
	if err := svc.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	for i := 0; i < 50; i++ {
		svc.Tap("k", identityRecord())
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("50 taps took %v against a stalled sink, the request path must not wait", d)
	}
	st := svc.Status()[0]
	if st.Dropped == 0 {
		t.Fatalf("status = %+v, want dropped events once the queue is full", st)
	}
	if st.Dropped+uint64(st.Queued) < 40 { //nolint:gosec // Queued is a small non-negative length
		t.Errorf("dropped %d + queued %d does not account for the offered events", st.Dropped, st.Queued)
	}
}

func TestServiceSinkFailureNeverAffectsTheCaller(t *testing.T) {
	bad := newReceiver(t, http.StatusInternalServerError)
	svc, _ := testService(t, webhookSpec("bad", bad, nil, true))
	for i := 0; i < 5; i++ {
		svc.Tap("k", identityRecord())
	}
	waitFor(t, func() bool { return svc.Status()[0].Failed >= 5 })
	st := svc.Status()[0]
	if st.LastErrorKind != KindServer || st.Sent != 0 {
		t.Errorf("status = %+v, want failed deliveries with kind server", st)
	}
}

func TestServiceHalfSucceededDelivery(t *testing.T) {
	good, bad := newReceiver(t, 200), newReceiver(t, http.StatusUnauthorized)
	svc, _ := testService(t, webhookSpec("a-good", good, nil, true), webhookSpec("b-bad", bad, nil, true))
	svc.Tap("k", identityRecord())
	waitFor(t, func() bool {
		s := svc.Status()
		return s[0].Sent == 1 && s[1].Failed == 1
	})
	s := svc.Status()
	if s[0].LastErrorKind != "" || s[0].LastSuccess.IsZero() {
		t.Errorf("healthy sink = %+v, want a last success and no error", s[0])
	}
	if s[1].LastErrorKind != KindAuth || !s[1].LastSuccess.IsZero() {
		t.Errorf("failing sink = %+v, want kind auth and no success", s[1])
	}
	// The failing sink recovers: a later delivery clears nothing but succeeds and sets last success.
	bad.setStatus(200)
	svc.Tap("k", identityRecord())
	waitFor(t, func() bool { return svc.Status()[1].Sent == 1 })
}

func TestServiceStatusNeverContainsSecrets(t *testing.T) {
	r := newReceiver(t, http.StatusForbidden)
	spec := webhookSpec("s", r, nil, true)
	spec.Secrets[SecretAuthHeader] = "Bearer topsecret"
	svc, _ := testService(t, spec)
	svc.Tap("k", identityRecord())
	waitFor(t, func() bool { return svc.Status()[0].Failed == 1 })
	b, _ := json.Marshal(svc.Status())
	for _, secret := range []string{"SECRETPATH", "topsecret", r.srv.URL} {
		if strings.Contains(string(b), secret) {
			t.Errorf("status leaks %q: %s", secret, b)
		}
	}
}

func TestServiceReloadStatesAndRetire(t *testing.T) {
	r := newReceiver(t, 200)
	good := webhookSpec("good", r, nil, true)
	off := webhookSpec("off", r, nil, true)
	off.Enabled = false
	blocked := SinkSpec{ID: "blocked", Name: "blocked", Kind: SinkWebhook, Enabled: true, Sample: 1, Secrets: urlSecret("ftp://example.com/x")}
	svc, src := testService(t, good, off, blocked)

	byName := map[string]SinkStatus{}
	for _, s := range svc.Status() {
		byName[s.Name] = s
	}
	if !byName["good"].Active || byName["off"].Active || byName["blocked"].Active || byName["blocked"].LastErrorKind != KindConfig {
		t.Fatalf("states = %+v", byName)
	}

	svc.Tap("k", identityRecord())
	waitFor(t, func() bool { return svc.Status()[1].Sent == 1 })
	if err := svc.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if svc.Status()[1].Sent != 1 {
		t.Error("an unchanged sink must keep its pipeline and counters across a reload")
	}
	src.set()
	if err := svc.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(svc.Status()) != 0 {
		t.Errorf("removed sinks should disappear from status: %+v", svc.Status())
	}
	svc.Tap("k", identityRecord())
}

func TestServiceTestDelivery(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		fields     []string
		wantOK     bool
		wantKind   string
		wantInBody []string
		wantNot    []string
	}{
		{"ok with default policy", 200, nil, true, "", []string{"export-test"}, []string{"192.0.2.", "client_ip", "user_agent"}},
		{"ok with opt-in truncated ip", 200, []string{FieldClientIP}, true, "", []string{"192.0.2.0"}, []string{"192.0.2.10"}},
		{"unauthorized", 401, nil, false, KindAuth, nil, nil},
		{"server error", 502, nil, false, KindServer, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newReceiver(t, tt.status)
			svc, _ := testService(t)
			res := svc.Test(context.Background(), webhookSpec("t", r, tt.fields, true))
			if res.OK != tt.wantOK || res.ErrorKind != tt.wantKind {
				t.Fatalf("result = %+v", res)
			}
			for _, w := range tt.wantInBody {
				if !strings.Contains(r.all(), w) {
					t.Errorf("body missing %q: %s", w, r.all())
				}
			}
			for _, w := range tt.wantNot {
				if strings.Contains(r.all(), w) {
					t.Errorf("body contains %q: %s", w, r.all())
				}
			}
		})
	}
}

func TestServiceTestDeliveryBadConfig(t *testing.T) {
	svc, _ := testService(t)
	res := svc.Test(context.Background(), SinkSpec{Kind: SinkWebhook, Secrets: urlSecret("ftp://example.com/x"), Sample: 1})
	if res.OK || res.ErrorKind != KindConfig {
		t.Fatalf("result = %+v, want a config failure", res)
	}
}

func TestTapRecoversFromPanickingResolver(t *testing.T) {
	r := newReceiver(t, 200)
	src := &fakeSource{specs: []SinkSpec{webhookSpec("s", r, nil, true)}}
	svc := NewService(src, HTTPOptions{AllowPrivate: true}, Options{QueueSize: 4, BatchSize: 1}, func(string) string { panic("resolver bug") }, nil, nil)
	t.Cleanup(func() { svc.Close(context.Background()) })
	if err := svc.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc.Tap("k", identityRecord())
}

func TestInstallWiresAccessLogAndToggle(t *testing.T) {
	r := newReceiver(t, 200)
	svc, src := testService(t, webhookSpec("s", r, []string{FieldUserAgent}, true))
	log := ingress.NewAccessLog(10)
	unsub := svc.Install(log)
	if !log.IdentityWanted() {
		t.Fatal("install should switch capture on for an opted-in sink")
	}
	log.Record("k", identityRecord())
	waitFor(t, func() bool { return strings.Contains(r.all(), "SecretAgent") })
	if got := log.Query([]string{"k"}, ingress.AccessFilter{}); len(got) != 1 || got[0].Identity != nil {
		t.Errorf("the ring must never store identity: %+v", got)
	}
	src.set()
	_ = svc.Reload(context.Background())
	if log.IdentityWanted() {
		t.Error("capture should switch off with the last opted-in sink")
	}
	unsub()
}

func TestBuildSinkRejectsBadSpecs(t *testing.T) {
	o := HTTPOptions{}
	tests := []struct {
		name string
		spec SinkSpec
	}{
		{"unknown kind", SinkSpec{Kind: "carrier", Sample: 1}},
		{"unknown field", SinkSpec{Kind: SinkWebhook, Fields: []string{"cookie"}, Sample: 1, Secrets: urlSecret("https://example.com/x")}},
		{"sample out of range", SinkSpec{Kind: SinkWebhook, Sample: 2, Secrets: urlSecret("https://example.com/x")}},
		{"posthog without token", SinkSpec{Kind: SinkPostHog, Sample: 1}},
		{"loopback webhook", SinkSpec{Kind: SinkWebhook, Sample: 1, Secrets: urlSecret("http://127.0.0.1/x")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := BuildSink(tt.spec, o); err == nil {
				t.Error("want an error")
			}
		})
	}
	if _, err := BuildSink(SinkSpec{Kind: SinkPostHog, Host: "eu", Sample: 1, Secrets: map[string]string{SecretToken: "t"}}, o); err != nil {
		t.Errorf("posthog eu: %v", err)
	}
}
