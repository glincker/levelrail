package certflow

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/domaindoctor"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name, issuer, source, owner string
		want                        Kind
	}{
		{"internal ca", "Caddy Local Authority - ECC Intermediate", "acme", "acme", KindInternal},
		{"owner internal", "whatever", "acme", "internal", KindInternal},
		{"staging", "(STAGING) Ersatz Edamame E5", "acme", "acme", KindStaging},
		{"lets encrypt R", "R11", "acme", "acme", KindLetsEncrypt},
		{"lets encrypt E", "E6", "acme", "acme", KindLetsEncrypt},
		{"lets encrypt name", "Let's Encrypt Authority X3", "acme", "acme", KindLetsEncrypt},
		{"custom wins", "R11", "custom", "custom", KindCustom},
		{"proxy", "R11", "proxy", "proxy", KindProxy},
		{"other ca", "ZeroSSL ECC Domain Secure Site CA", "acme", "acme", KindOther},
		{"empty", "", "acme", "acme", KindNone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.issuer, tc.source, tc.owner); got != tc.want {
				t.Fatalf("Classify = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildRenewal(t *testing.T) {
	nb := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	fa := nb.Add(24 * time.Hour)
	if r := BuildRenewal(KindLetsEncrypt, nb, fa, "boom", true); r == nil || r.Result != RenewalFailed || r.Error != "boom" || !r.At.Equal(fa) {
		t.Fatalf("failed renewal = %+v", r)
	}
	if r := BuildRenewal(KindLetsEncrypt, nb, time.Time{}, "", false); r == nil || r.Result != RenewalOK || !r.At.Equal(nb) {
		t.Fatalf("ok renewal = %+v", r)
	}
	if r := BuildRenewal(KindInternal, nb, time.Time{}, "", false); r != nil {
		t.Fatalf("internal certs renew silently, got %+v", r)
	}
}

func TestFailDaysEnv(t *testing.T) {
	if FailDays() != 14 {
		t.Fatalf("default = %d", FailDays())
	}
	t.Setenv(EnvFailDays, "21")
	if FailDays() != 21 {
		t.Fatalf("env = %d", FailDays())
	}
	t.Setenv(EnvFailDays, "junk")
	if FailDays() != 14 {
		t.Fatalf("bad env = %d", FailDays())
	}
}

func check(id, title string) domaindoctor.Check {
	return domaindoctor.Check{ID: id, State: domaindoctor.StatePass, Title: title, Detail: "d", Fix: &domaindoctor.Fix{Summary: "fix " + id}}
}

func goodReport() domaindoctor.Report {
	return domaindoctor.Report{Probed: true, Checks: []domaindoctor.Check{
		check(domaindoctor.IDDNSResolves, "resolves"),
		check(domaindoctor.IDDNSPointsHere, "points"),
		check(domaindoctor.IDPort80, "port 80"),
		check(domaindoctor.IDCAA, "caa"),
	}}
}

func with(rep domaindoctor.Report, id string, st domaindoctor.State) domaindoctor.Report {
	out := domaindoctor.Report{Probed: rep.Probed, ProbeNote: rep.ProbeNote}
	for _, c := range rep.Checks {
		if c.ID == id {
			c.State = st
		}
		out.Checks = append(out.Checks, c)
	}
	return out
}

func item(r Result, code string) Item {
	for _, it := range r.Items {
		if it.Code == code {
			return it
		}
	}
	return Item{}
}

func TestEvaluate(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	base := func() Input {
		return Input{Domain: "app.shop.dev", Email: "me@shop.dev", Report: goodReport(), Limits: Limits{FailedPerHour: 5, DuplicatePerWeek: 5}, Now: now}
	}
	tests := []struct {
		name      string
		mut       func(*Input)
		wantReady bool
		code      string
		state     ItemState
	}{
		{"all good", func(*Input) {}, true, CodeDNS, StatePass},
		{"dns missing", func(i *Input) { i.Report = with(i.Report, domaindoctor.IDDNSPointsHere, domaindoctor.StateFail) }, false, CodeDNS, StateFail},
		{"port 80 closed", func(i *Input) { i.Report = with(i.Report, domaindoctor.IDPort80, domaindoctor.StateFail) }, false, CodePorts, StateFail},
		{"port 80 not probed warns", func(i *Input) {
			i.Report = with(i.Report, domaindoctor.IDPort80, domaindoctor.StateSkipped)
			i.Report.ProbeNote = "does not resolve here"
		}, true, CodePorts, StateWarn},
		{"caa blocks", func(i *Input) { i.Report = with(i.Report, domaindoctor.IDCAA, domaindoctor.StateFail) }, false, CodeCAA, StateFail},
		{"caa unreadable warns", func(i *Input) { i.Report = with(i.Report, domaindoctor.IDCAA, domaindoctor.StateUnavailable) }, true, CodeCAA, StateWarn},
		{"no email", func(i *Input) { i.Email = "" }, false, CodeEmail, StateFail},
		{"default email used", func(i *Input) { i.Email, i.DefaultEmail = "", "admin@shop.dev" }, true, CodeEmail, StatePass},
		{"reserved email", func(i *Input) { i.Email = "a@example.com" }, false, CodeEmail, StateFail},
		{"bad email", func(i *Input) { i.Email = "Ops <a@b.dev>" }, false, CodeEmail, StateFail},
		{"unissuable name", func(i *Input) { i.Domain = "app.local" }, false, CodeIssuable, StateFail},
		{"upstream proxy", func(i *Input) { i.TLSUpstream = true }, false, CodeUpstream, StateFail},
		{"failed validations", func(i *Input) {
			for n := 0; n < 5; n++ {
				i.History.Failed = append(i.History.Failed, now.Add(-time.Duration(n+1)*5*time.Minute))
			}
		}, false, CodeRateLimit, StateFail},
		{"staging skips rate limit and warns", func(i *Input) {
			i.Staging = true
			i.History.Failed = []time.Time{now, now, now, now, now}
		}, true, CodeStaging, StateWarn},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := base()
			tc.mut(&in)
			res := Evaluate(in)
			if res.Ready != tc.wantReady {
				t.Fatalf("Ready = %v, want %v: %+v", res.Ready, tc.wantReady, res.Items)
			}
			if got := item(res, tc.code); got.State != tc.state {
				t.Fatalf("%s state = %q, want %q (%+v)", tc.code, got.State, tc.state, got)
			}
		})
	}
}

func TestEvaluateStagingSkipsRateLimitItem(t *testing.T) {
	in := Input{Domain: "a.shop.dev", Email: "m@shop.dev", Staging: true, Report: goodReport(), Now: time.Now()}
	res := Evaluate(in)
	if got := item(res, CodeRateLimit); got.State != StateSkip {
		t.Fatalf("rate limit on staging = %q", got.State)
	}
	if res.NextSafeAt != nil {
		t.Fatalf("staging must not report a next safe time")
	}
}

func TestRateLimitNextSafeTime(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	lim := Limits{FailedPerHour: 3, DuplicatePerWeek: 2}
	tests := []struct {
		name   string
		snap   Snapshot
		caErr  string
		caAt   time.Time
		blocks bool
		reason string
		next   time.Time
	}{
		{"clear", Snapshot{}, "", time.Time{}, false, "", time.Time{}},
		{"failures below limit", Snapshot{Failed: []time.Time{now.Add(-10 * time.Minute), now.Add(-5 * time.Minute)}}, "", time.Time{}, false, "", time.Time{}},
		{"failures at limit: oldest + 1h", Snapshot{Failed: []time.Time{now.Add(-50 * time.Minute), now.Add(-20 * time.Minute), now.Add(-5 * time.Minute)}}, "", time.Time{}, true, ReasonFailedValidations, now.Add(10 * time.Minute)},
		{"old failures expire", Snapshot{Failed: []time.Time{now.Add(-3 * time.Hour), now.Add(-2 * time.Hour), now.Add(-90 * time.Minute)}}, "", time.Time{}, false, "", time.Time{}},
		{"duplicates: oldest + 7d", Snapshot{Issued: []time.Time{now.Add(-48 * time.Hour), now.Add(-24 * time.Hour)}}, "", time.Time{}, true, ReasonDuplicates, now.Add(-48 * time.Hour).Add(7 * 24 * time.Hour)},
		{"ca said rate limited", Snapshot{}, "urn:ietf:params:acme:error:rateLimited", now.Add(-20 * time.Minute), true, ReasonCARateLimited, now.Add(40 * time.Minute)},
		{"old ca rate limit forgotten", Snapshot{}, "rateLimited", now.Add(-3 * time.Hour), false, "", time.Time{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := tc.snap.Check(lim, now, tc.caErr, tc.caAt)
			if v.Blocked != tc.blocks || v.Reason != tc.reason || !v.NextSafeAt.Equal(tc.next) {
				t.Fatalf("verdict = %+v, want blocked=%v reason=%q next=%v", v, tc.blocks, tc.reason, tc.next)
			}
		})
	}
}

func TestLedgerDedupesAndSnapshots(t *testing.T) {
	var l Ledger
	now := time.Now()
	l.RecordFailure("a.dev", now, now)
	l.RecordFailure("a.dev", now, now)
	l.RecordIssued("a.dev", now.Add(-time.Hour), now)
	s := l.Snapshot("a.dev")
	if len(s.Failed) != 1 || len(s.Issued) != 1 {
		t.Fatalf("snapshot = %+v", s)
	}
	l.MarkStarted("a.dev", now)
	if !l.StartedAt("a.dev").Equal(now) || !l.StartedAt("b.dev").IsZero() {
		t.Fatal("started tracking is per domain")
	}
}

func TestWatchdogBackoffAndRecovery(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	var stuck = []Candidate{{Domain: "a.dev", Reason: "renewal_failed"}}
	var renewed []string
	renewErr := error(nil)
	w := NewWatchdog(WatchdogDeps{
		Scan:  func(context.Context) ([]Candidate, error) { return stuck, nil },
		Renew: func(_ context.Context, c Candidate) error { renewed = append(renewed, c.Domain); return renewErr },
		Now:   func() time.Time { return now },
	})
	w.base = 30 * time.Minute
	ctx := context.Background()

	w.Tick(ctx)
	if len(renewed) != 1 {
		t.Fatalf("first tick renewed %d", len(renewed))
	}
	now = now.Add(10 * time.Minute)
	w.Tick(ctx)
	if len(renewed) != 1 {
		t.Fatalf("retry before backoff elapsed: %d", len(renewed))
	}
	st, ok := w.State("a.dev")
	if !ok || st.Attempts != 1 || !st.NextAt.Equal(now.Add(20*time.Minute)) {
		t.Fatalf("state = %+v", st)
	}
	now = now.Add(21 * time.Minute)
	renewErr = errors.New("nudge failed")
	w.Tick(ctx)
	st, _ = w.State("a.dev")
	if len(renewed) != 2 || st.Attempts != 2 || st.LastErr == "" {
		t.Fatalf("second attempt: renewed=%d state=%+v", len(renewed), st)
	}
	if got := st.NextAt.Sub(now); got != time.Hour {
		t.Fatalf("backoff after 2 attempts = %v, want 1h", got)
	}

	stuck = nil
	w.Tick(ctx)
	if _, ok := w.State("a.dev"); ok {
		t.Fatal("recovered domain must be forgotten")
	}
}

func TestWatchdogSkipsBlockedAndCapsBackoff(t *testing.T) {
	now := time.Now()
	calls := 0
	w := NewWatchdog(WatchdogDeps{
		Scan:  func(context.Context) ([]Candidate, error) { return []Candidate{{Domain: "b.dev", Blocked: true}}, nil },
		Renew: func(context.Context, Candidate) error { calls++; return nil },
		Now:   func() time.Time { return now },
	})
	w.Tick(context.Background())
	if calls != 0 {
		t.Fatal("a rate limited domain must not be retried")
	}
	if got := backoff(30*time.Minute, 20); got != maxWatchdogBackoff {
		t.Fatalf("backoff cap = %v", got)
	}
	if got := backoff(30*time.Minute, 1); got != 30*time.Minute {
		t.Fatalf("first backoff = %v", got)
	}
}

func TestWatchdogScanErrorIsQuiet(t *testing.T) {
	w := NewWatchdog(WatchdogDeps{
		Scan:  func(context.Context) ([]Candidate, error) { return nil, errors.New("down") },
		Renew: func(context.Context, Candidate) error { t.Fatal("must not renew"); return nil },
	})
	w.Tick(context.Background())
}

func TestStagingConflictMatrix(t *testing.T) {
	mk := func(k Kind, expired, stalled bool) []Existing {
		return []Existing{{Domain: "a.dev", Kind: k, Expired: expired, Stalled: stalled}}
	}
	tests := []struct {
		name     string
		existing []Existing
		staging  bool
		blocked  bool
	}{
		{"internal, staging", mk(KindInternal, false, false), true, false},
		{"internal, production", mk(KindInternal, false, false), false, false},
		{"staging cert, staging", mk(KindStaging, false, false), true, false},
		{"staging cert, production", mk(KindStaging, false, false), false, false},
		{"letsencrypt valid, staging", mk(KindLetsEncrypt, false, false), true, true},
		{"letsencrypt valid, production", mk(KindLetsEncrypt, false, false), false, false},
		{"letsencrypt expired, staging", mk(KindLetsEncrypt, true, false), true, false},
		{"letsencrypt stalled, staging", mk(KindLetsEncrypt, false, true), true, false},
		{"custom valid, staging", mk(KindCustom, false, false), true, true},
		{"custom valid, production", mk(KindCustom, false, false), false, false},
		{"proxy valid, staging", mk(KindProxy, false, false), true, true},
		{"proxy valid, production", mk(KindProxy, false, false), false, false},
		{"other CA valid, staging", mk(KindOther, false, false), true, true},
		{"another domain's custom cert is not at risk", []Existing{{Domain: "b.dev", Kind: KindCustom}}, true, false},
		{"another domain's letsencrypt cert is at risk (CA switch is server-wide)", []Existing{{Domain: "b.dev", Kind: KindLetsEncrypt}}, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := Input{Domain: "a.dev", Email: "m@shop.dev", Staging: tc.staging, Existing: tc.existing, Limits: Limits{FailedPerHour: 5, DuplicatePerWeek: 5}, Report: goodReport(), Now: time.Now()}
			res := Evaluate(in)
			got := item(res, CodeReplaces)
			if tc.blocked != (got.State == StateFail) || tc.blocked == res.Ready {
				t.Fatalf("blocked = %v, item = %+v, ready = %v", tc.blocked, got, res.Ready)
			}
			if tc.blocked && (got.Fix == "" || !strings.Contains(got.Detail, "Next step")) {
				t.Fatalf("blocked item needs a message and next step: %+v", got)
			}
			if err := StagingConflict("a.dev", tc.existing); tc.staging && (err != nil) != tc.blocked {
				t.Fatalf("StagingConflict = %v", err)
			}
		})
	}
}
