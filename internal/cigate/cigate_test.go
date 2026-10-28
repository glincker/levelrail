package cigate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

type fakeProvider struct {
	rounds [][]Check
	errs   []error
	calls  int
}

func (f *fakeProvider) Checks(_ context.Context, _ string) ([]Check, error) {
	i := f.calls
	f.calls++
	if i < len(f.errs) && f.errs[i] != nil {
		return nil, f.errs[i]
	}
	if i >= len(f.rounds) {
		i = len(f.rounds) - 1
	}
	return f.rounds[i], nil
}

type fakeClock struct {
	now    time.Time
	sleeps []time.Duration
}

func (c *fakeClock) cfg() Config {
	return Config{
		Timeout: time.Minute, PollInterval: 10 * time.Second, NoChecksWait: 20 * time.Second,
		Now: func() time.Time { return c.now },
		Sleep: func(_ context.Context, d time.Duration) error {
			c.sleeps = append(c.sleeps, d)
			c.now = c.now.Add(d)
			return nil
		},
	}
}

func newClock() *fakeClock { return &fakeClock{now: time.Unix(1_700_000_000, 0)} }

func TestWaitPass(t *testing.T) {
	p := &fakeProvider{rounds: [][]Check{
		{{"test", Pending}, {"lint", Passed}},
		{{"test", Passed}, {"lint", Passed}},
	}}
	clk := newClock()
	res, err := Wait(context.Background(), p, "abc", Policy{}, clk.cfg())
	if err != nil || res.Outcome != OutcomePass {
		t.Fatalf("got %+v err %v", res, err)
	}
	if p.calls != 2 {
		t.Fatalf("calls = %d", p.calls)
	}
}

func TestWaitFailNamesChecks(t *testing.T) {
	p := &fakeProvider{rounds: [][]Check{{{"test", Failed}, {"lint", Passed}, {"e2e", Pending}}}}
	res, _ := Wait(context.Background(), p, "abc", Policy{}, newClock().cfg())
	if res.Outcome != OutcomeFail || len(res.Failing) != 1 || res.Failing[0] != "test" {
		t.Fatalf("got %+v", res)
	}
}

func TestWaitTimeout(t *testing.T) {
	p := &fakeProvider{rounds: [][]Check{{{"test", Pending}}}}
	clk := newClock()
	res, err := Wait(context.Background(), p, "abc", Policy{}, clk.cfg())
	if err != nil || res.Outcome != OutcomeTimeout || len(res.Pending) != 1 {
		t.Fatalf("got %+v err %v", res, err)
	}
	if clk.now.Sub(time.Unix(1_700_000_000, 0)) > time.Minute {
		t.Fatalf("slept past the timeout: %v", clk.sleeps)
	}
}

func TestWaitIgnoresOwnCheck(t *testing.T) {
	p := &fakeProvider{rounds: [][]Check{{{"Acme deploy (production)", Pending}, {"test", Passed}}}}
	res, _ := Wait(context.Background(), p, "abc", Policy{IgnorePrefixes: []string{"acme deploy"}}, newClock().cfg())
	if res.Outcome != OutcomePass {
		t.Fatalf("own pending check blocked the gate: %+v", res)
	}
}

func TestWaitRequiredMissingThenPass(t *testing.T) {
	p := &fakeProvider{rounds: [][]Check{
		{{"lint", Passed}},
		{{"lint", Passed}, {"test", Passed}},
	}}
	res, _ := Wait(context.Background(), p, "abc", Policy{Required: []string{"test"}}, newClock().cfg())
	if res.Outcome != OutcomePass || p.calls != 2 {
		t.Fatalf("got %+v calls %d", res, p.calls)
	}
}

func TestWaitRequiredIgnoresOtherFailures(t *testing.T) {
	p := &fakeProvider{rounds: [][]Check{{{"test", Passed}, {"flaky", Failed}}}}
	res, _ := Wait(context.Background(), p, "abc", Policy{Required: []string{"test"}}, newClock().cfg())
	if res.Outcome != OutcomePass {
		t.Fatalf("got %+v", res)
	}
}

func TestWaitNoChecksPassAfterGrace(t *testing.T) {
	p := &fakeProvider{rounds: [][]Check{{}}}
	res, _ := Wait(context.Background(), p, "abc", Policy{}, newClock().cfg())
	if res.Outcome != OutcomePass || p.calls < 2 {
		t.Fatalf("got %+v calls %d", res, p.calls)
	}
}

func TestWaitRespectsRateLimit(t *testing.T) {
	p := &fakeProvider{
		rounds: [][]Check{{{"test", Passed}}},
		errs:   []error{&RateLimitError{RetryAfter: 40 * time.Second}},
	}
	clk := newClock()
	res, _ := Wait(context.Background(), p, "abc", Policy{}, clk.cfg())
	if res.Outcome != OutcomePass || len(clk.sleeps) == 0 || clk.sleeps[0] != 40*time.Second {
		t.Fatalf("got %+v sleeps %v", res, clk.sleeps)
	}
}

func TestGitHubChecks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/repos/o/r/commits/sha1/check-runs":
			_, _ = w.Write([]byte(`{"check_runs":[{"name":"build","status":"completed","conclusion":"success"},{"name":"e2e","status":"in_progress"},{"name":"vet","status":"completed","conclusion":"timed_out"}]}`))
		case "/repos/o/r/commits/sha1/status":
			_, _ = w.Write([]byte(`{"statuses":[{"context":"ci/legacy","state":"failure"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	g := &GitHub{HTTP: srv.Client(), BaseURL: srv.URL, Token: "tok", Owner: "o", Repo: "r"}
	got, err := g.Checks(context.Background(), "sha1")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]State{"build": Passed, "e2e": Pending, "vet": Failed, "ci/legacy": Failed}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for _, c := range got {
		if want[c.Name] != c.State {
			t.Fatalf("%s = %v", c.Name, c.State)
		}
	}
}

func TestGitHubRateLimit(t *testing.T) {
	reset := time.Now().Add(90 * time.Second).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	g := &GitHub{HTTP: srv.Client(), BaseURL: srv.URL, Token: "tok", Owner: "o", Repo: "r"}
	_, err := g.Checks(context.Background(), "sha1")
	var rl *RateLimitError
	ok := errors.As(err, &rl)
	if !ok || rl.RetryAfter < 60*time.Second {
		t.Fatalf("err = %v", err)
	}
}
