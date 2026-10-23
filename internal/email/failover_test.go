package email

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const (
	fakeA Backend = "fake_a"
	fakeB Backend = "fake_b"
)

var (
	fakeMu    sync.Mutex
	fakeCalls = map[Backend]int{}
	fakeFn    = map[Backend]func(call int) error{}
)

func init() {
	for _, b := range []Backend{fakeA, fakeB} {
		Register(b, func(cfg Config) (Sender, error) { return fakeSender{backend: cfg.Backend}, nil })
	}
}

type fakeSender struct{ backend Backend }

func (f fakeSender) Send(context.Context, string, string, string) error {
	fakeMu.Lock()
	fakeCalls[f.backend]++
	n := fakeCalls[f.backend]
	fn := fakeFn[f.backend]
	fakeMu.Unlock()
	if fn == nil {
		return nil
	}
	return fn(n)
}

func setFakes(a, b func(int) error) {
	fakeMu.Lock()
	defer fakeMu.Unlock()
	fakeCalls = map[Backend]int{}
	fakeFn = map[Backend]func(int) error{fakeA: a, fakeB: b}
}

func calls(b Backend) int {
	fakeMu.Lock()
	defer fakeMu.Unlock()
	return fakeCalls[b]
}

func always(err error) func(int) error { return func(int) error { return err } }

type foRecorder struct {
	mu         sync.Mutex
	deliveries []Delivery
	sent       int
	health     map[string]string
}

func (r *foRecorder) RecordDelivery(_ context.Context, d Delivery) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deliveries = append(r.deliveries, d)
	return nil
}

func (r *foRecorder) CountSent(context.Context, time.Time) (int, error) { return r.sent, nil }

func (r *foRecorder) RecordProviderResult(_ context.Context, id string, ok bool, kind FailureKind, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.health == nil {
		r.health = map[string]string{}
	}
	if ok {
		r.health[id] = "ok"
	} else {
		r.health[id] = string(kind)
	}
	return nil
}

func twoProviders(limits Limits) ProvidersLoader {
	return func(context.Context) ([]ProviderConfig, error) {
		return []ProviderConfig{
			{ID: "p", Config: Config{Backend: fakeA, Limits: limits}},
			{ID: "b", Config: Config{Backend: fakeB}},
		}, nil
	}
}

func newTwo(rec *foRecorder) *DynamicSender {
	d := NewDynamicSender(nil).WithProviders(twoProviders(Limits{})).WithRecorder(rec).WithProviderHealth(rec).WithRetry(3, time.Nanosecond)
	return d
}

var errBody = errors.New("body was sent then connection dropped")

func TestFailoverEligible(t *testing.T) {
	dialErr := &net.OpError{Op: "dial", Err: errors.New("no route")}
	readTimeout := &net.OpError{Op: "read", Err: timeoutErr{}}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"dial error", dialErr, true},
		{"conn refused", fmt.Errorf("x: %w", syscall.ECONNREFUSED), true},
		{"pre-send timeout", preSend(readTimeout), true},
		{"post-body timeout", readTimeout, false},
		{"post-body reset", errBody, false},
		{"auth", &textproto.Error{Code: 535, Msg: "auth failed"}, true},
		{"tls pre-send", preSend(errors.New("starttls: x509: certificate signed by unknown authority")), true},
		{"sender not verified", &HTTPError{Provider: "postmark", Status: 422, Body: "Sender signature not found"}, true},
		{"region mismatch", errors.New("email identity is not verified in region us-east-1"), true},
		{"http 503", &HTTPError{Provider: "x", Status: 503, Body: "busy"}, true},
		{"http 429", &HTTPError{Provider: "x", Status: 429, Body: "slow"}, true},
		{"http 401", &HTTPError{Provider: "x", Status: 401, Body: "bad key"}, true},
		{"http 422 content", &HTTPError{Provider: "x", Status: 422, Body: "invalid subject"}, false},
		{"http 400", &HTTPError{Provider: "x", Status: 400, Body: "bad request"}, false},
		{"recipient rejected rcpt", preSend(&textproto.Error{Code: 550, Msg: "mailbox unavailable"}), false},
		{"smtp 452 temp", preSend(&textproto.Error{Code: 452, Msg: "too many recipients"}), false},
		{"own rate limit", ErrRateLimited, false},
		{"canceled", context.Canceled, false},
		{"not configured", ErrNotConfigured, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FailoverEligible(tt.err); got != tt.want {
				t.Errorf("FailoverEligible(%v) = %v, want %v (kind %q)", tt.err, got, tt.want, Classify(tt.err))
			}
		})
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestRun_FailoverMatrix(t *testing.T) {
	tests := []struct {
		name         string
		primaryErr   error
		wantFailover bool
		wantPrimary  int
	}{
		{"auth fails over, not retried", &textproto.Error{Code: 535, Msg: "bad"}, true, 1},
		{"dial error fails over", &net.OpError{Op: "dial", Err: errors.New("down")}, true, 1},
		{"503 retried then fails over", &HTTPError{Provider: "x", Status: 503, Body: "busy"}, true, 3},
		{"429 retried then fails over", &HTTPError{Provider: "x", Status: 429, Body: "slow"}, true, 3},
		{"sender not verified", &HTTPError{Provider: "x", Status: 403, Body: "domain is not verified"}, true, 1},
		{"recipient rejection stays", preSend(&textproto.Error{Code: 550, Msg: "no such user"}), false, 1},
		{"content error stays", &HTTPError{Provider: "x", Status: 422, Body: "invalid"}, false, 1},
		{"ambiguous drop stays", errBody, false, 1},
		{"post-body timeout stays", &net.OpError{Op: "read", Err: timeoutErr{}}, false, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setFakes(always(tt.primaryErr), nil)
			rec := &foRecorder{}
			res, err := newTwo(rec).SendDetailed(context.Background(), "u@example.com", "s", "b")
			if got := calls(fakeB) > 0; got != tt.wantFailover {
				t.Fatalf("backup used = %v, want %v", got, tt.wantFailover)
			}
			if calls(fakeA) != tt.wantPrimary {
				t.Errorf("primary attempts = %d, want %d", calls(fakeA), tt.wantPrimary)
			}
			if tt.wantFailover {
				if err != nil || res.Provider != string(fakeB) {
					t.Fatalf("res=%+v err=%v, want success via backup", res, err)
				}
				last := rec.deliveries[len(rec.deliveries)-1]
				if last.Status != DeliverySent || last.ProviderID != "b" || last.FailoverFrom != "p" {
					t.Errorf("backup delivery = %+v", last)
				}
				if rec.health["p"] == "" || rec.health["b"] != "ok" {
					t.Errorf("health = %v", rec.health)
				}
			} else if err == nil {
				t.Fatal("want the primary error")
			}
		})
	}
}

func TestRun_BothFailReturnsCombinedError(t *testing.T) {
	setFakes(always(&textproto.Error{Code: 535, Msg: "primary auth"}), always(&HTTPError{Provider: "b", Status: 500, Body: "backup down"}))
	rec := &foRecorder{}
	_, err := newTwo(rec).SendDetailed(context.Background(), "u@example.com", "s", "b")
	if err == nil || !strings.Contains(err.Error(), "primary auth") || !strings.Contains(err.Error(), "backup down") {
		t.Fatalf("err = %v, want both failures", err)
	}
	var se *SendError
	if !errors.As(err, &se) || se.Kind != FailAuth || se.Attempts != 1+3 {
		t.Errorf("SendError = %+v", se)
	}
}

func TestRun_CapsStayGlobal(t *testing.T) {
	setFakes(always(nil), always(nil))
	rec := &foRecorder{sent: 5}
	d := NewDynamicSender(nil).WithProviders(twoProviders(Limits{PerMinute: 5})).WithRecorder(rec)
	_, err := d.SendDetailed(context.Background(), "u@example.com", "s", "b")
	if Classify(err) != FailRateLimited || calls(fakeA)+calls(fakeB) != 0 {
		t.Fatalf("err=%v calls a=%d b=%d, want capped before any provider", err, calls(fakeA), calls(fakeB))
	}
}

func TestRun_TestSendsNeverFailOver(t *testing.T) {
	for name, ctx := range map[string]context.Context{
		"WithoutRetry":    WithoutRetry(context.Background()),
		"WithoutFailover": WithoutFailover(context.Background()),
	} {
		t.Run(name, func(t *testing.T) {
			setFakes(always(&textproto.Error{Code: 535, Msg: "bad"}), nil)
			if _, err := newTwo(&foRecorder{}).SendDetailed(ctx, "u@example.com", "s", "b"); err == nil {
				t.Fatal("want error")
			}
			if calls(fakeB) != 0 {
				t.Error("test send reached the backup")
			}
		})
	}
}

func TestRun_CanceledContextDoesNotFailOver(t *testing.T) {
	setFakes(always(&net.OpError{Op: "dial", Err: context.Canceled}), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = newTwo(&foRecorder{}).SendDetailed(ctx, "u@example.com", "s", "b")
	if calls(fakeB) != 0 {
		t.Error("failed over after cancel")
	}
}

func TestBreaker_OpenHalfOpenClose(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rec := &foRecorder{}
	d := newTwo(rec).WithFailover(2, time.Minute)
	d.breaker.now = func() time.Time { return now }
	authErr := &textproto.Error{Code: 535, Msg: "bad"}
	setFakes(always(authErr), nil)

	send := func() { _, _ = d.SendDetailed(context.Background(), "u@example.com", "s", "b") }
	send()
	send() // second consecutive failure opens the circuit
	if calls(fakeA) != 2 {
		t.Fatalf("primary calls = %d, want 2", calls(fakeA))
	}
	send() // open: skipped, straight to the backup
	if calls(fakeA) != 2 || calls(fakeB) != 3 {
		t.Fatalf("while open: a=%d b=%d, want 2 and 3", calls(fakeA), calls(fakeB))
	}
	last := rec.deliveries[len(rec.deliveries)-1]
	if last.FailoverFrom != "p" || last.ProviderID != "b" {
		t.Errorf("skipped-primary delivery = %+v", last)
	}

	now = now.Add(time.Minute) // cooldown over: one probe, which fails and reopens
	send()
	if calls(fakeA) != 3 {
		t.Fatalf("probe calls = %d, want 3", calls(fakeA))
	}
	send()
	if calls(fakeA) != 3 {
		t.Fatal("reopened circuit still let traffic through")
	}

	now = now.Add(time.Minute) // second probe succeeds and closes
	setFakes(nil, nil)
	send()
	send()
	if calls(fakeA) != 2 {
		t.Errorf("after close primary calls = %d, want 2 (probe + normal)", calls(fakeA))
	}
}

func TestBreaker_HalfOpenAdmitsSingleProbe(t *testing.T) {
	now := time.Now()
	b := newBreaker(1, time.Second)
	b.now = func() time.Time { return now }
	b.report("p", outcomeFail)
	if b.allow("p") {
		t.Fatal("open circuit allowed")
	}
	now = now.Add(2 * time.Second)
	if !b.allow("p") || b.allow("p") {
		t.Fatal("want exactly one half-open probe")
	}
	b.report("p", outcomeOK)
	if !b.allow("p") {
		t.Fatal("closed circuit should allow all")
	}
}

func TestBreaker_NonFailoverFailureResetsCount(t *testing.T) {
	setFakes(nil, nil)
	d := newTwo(&foRecorder{}).WithFailover(2, time.Minute)
	for _, e := range []error{
		&textproto.Error{Code: 535, Msg: "x"},
		preSend(&textproto.Error{Code: 550, Msg: "recipient"}),
		&textproto.Error{Code: 535, Msg: "x"},
	} {
		setFakes(always(e), nil)
		_, _ = d.SendDetailed(context.Background(), "u@example.com", "s", "b")
	}
	if !d.breaker.allow("p") {
		t.Error("non-consecutive failures opened the circuit")
	}
}

func TestRun_NoSecretsInLogsOrDeliveries(t *testing.T) {
	const secret = "sk_live_SEEDED_SECRET_123" //nolint:gosec // seeded test value
	const addr = "victim@example.com"
	setFakes(always(&HTTPError{Provider: "x", Status: 503, Body: "upstream said Bearer " + secret + " for " + addr + " " + strings.Repeat("z", 2000)}), nil)
	var logs bytes.Buffer
	rec := &foRecorder{}
	d := NewDynamicSender(nil).WithRecorder(rec).WithLogger(slog.New(slog.NewTextHandler(&logs, nil))).WithRetry(1, time.Nanosecond).
		WithProviders(func(context.Context) ([]ProviderConfig, error) {
			return []ProviderConfig{
				{ID: "p", Config: Config{Backend: fakeA, API: &APIConfig{APIKey: secret}}},
				{ID: "b", Config: Config{Backend: fakeB}},
			}, nil
		})
	if _, err := d.SendDetailed(context.Background(), addr, "s", "b"); err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	all.WriteString(logs.String())
	for _, del := range rec.deliveries {
		all.WriteString(del.Error)
		if len(del.Error) > maxProviderText+100 {
			t.Errorf("stored error length %d not truncated", len(del.Error))
		}
	}
	for _, leak := range []string{secret, addr} {
		if strings.Contains(all.String(), leak) {
			t.Errorf("%q leaked into logs or delivery log", leak)
		}
	}
	for _, field := range []string{"provider_id=p", "backend=fake_a", "failed_over_to=b", "attempt=1"} {
		if !strings.Contains(logs.String(), field) {
			t.Errorf("log missing %s: %s", field, logs.String())
		}
	}
}

func TestRun_SingleProviderParity(t *testing.T) {
	cases := map[string]error{
		"ok":      nil,
		"auth":    &textproto.Error{Code: 535, Msg: "bad"},
		"503":     &HTTPError{Provider: "x", Status: 503, Body: "busy"},
		"dropped": errBody,
	}
	for name, perr := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := Config{Backend: fakeA}
			run := func(d *DynamicSender) ([]Delivery, int, error) {
				setFakes(always(perr), nil)
				rec := &foRecorder{}
				_, err := d.WithRecorder(rec).WithRetry(3, time.Nanosecond).SendDetailed(context.Background(), "u@example.com", "s", "b")
				return rec.deliveries, calls(fakeA), err
			}
			oldD, oldCalls, oldErr := run(NewDynamicSender(func(context.Context) (Config, error) { return cfg, nil }))
			newD, newCalls, newErr := run(NewDynamicSender(nil).WithProviders(func(context.Context) ([]ProviderConfig, error) {
				return []ProviderConfig{{ID: "only", Config: cfg}}, nil
			}))
			if oldCalls != newCalls || fmt.Sprint(oldErr) != fmt.Sprint(newErr) || Classify(oldErr) != Classify(newErr) || len(oldD) != len(newD) {
				t.Fatalf("diverged: calls %d/%d err %v/%v deliveries %d/%d", oldCalls, newCalls, oldErr, newErr, len(oldD), len(newD))
			}
			for i := range oldD {
				a, b := oldD[i], newD[i]
				a.At, b.At, b.ProviderID = time.Time{}, time.Time{}, ""
				if a != b {
					t.Errorf("delivery %d: %+v vs %+v", i, a, b)
				}
			}
		})
	}
}
