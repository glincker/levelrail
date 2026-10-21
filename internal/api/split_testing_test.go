package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/splittest"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeSplitStore struct {
	tests    []store.SplitTest
	stats    map[string]map[string]uint64
	branches []store.BranchDeploy
	urls     []store.DeployURL
	endErr   error
}

func (f *fakeSplitStore) CreateSplitTest(_ context.Context, t store.SplitTest) error {
	for _, x := range f.tests {
		if x.AppName == t.AppName && x.Status == splittest.StatusRunning {
			return store.ErrSplitTestRunning
		}
	}
	t.CreatedAt = time.Now()
	f.tests = append(f.tests, t)
	return nil
}

func (f *fakeSplitStore) GetRunningSplitTest(_ context.Context, app string) (store.SplitTest, bool, error) {
	for _, t := range f.tests {
		if t.AppName == app && t.Status == splittest.StatusRunning {
			return t, true, nil
		}
	}
	return store.SplitTest{}, false, nil
}

func (f *fakeSplitStore) GetLatestSplitTest(_ context.Context, app string) (store.SplitTest, bool, error) {
	for i := len(f.tests) - 1; i >= 0; i-- {
		if f.tests[i].AppName == app {
			return f.tests[i], true, nil
		}
	}
	return store.SplitTest{}, false, nil
}

func (f *fakeSplitStore) ListRunningSplitTests(context.Context) ([]store.SplitTest, error) {
	var out []store.SplitTest
	for _, t := range f.tests {
		if t.Status == splittest.StatusRunning {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeSplitStore) UpdateSplitTestVariants(_ context.Context, id string, vs []store.SplitVariant) error {
	for i := range f.tests {
		if f.tests[i].ID == id {
			f.tests[i].Variants = vs
		}
	}
	return nil
}

func (f *fakeSplitStore) EndSplitTest(_ context.Context, id, reason string) error {
	if f.endErr != nil {
		return f.endErr
	}
	for i := range f.tests {
		if f.tests[i].ID == id {
			f.tests[i].Status, f.tests[i].EndReason, f.tests[i].EndedAt = splittest.StatusStopped, reason, time.Now()
		}
	}
	return nil
}

func (f *fakeSplitStore) AddSplitStats(_ context.Context, testID string, counts map[string]uint64, _ time.Time) error {
	if f.stats == nil {
		f.stats = map[string]map[string]uint64{}
	}
	if f.stats[testID] == nil {
		f.stats[testID] = map[string]uint64{}
	}
	for k, n := range counts {
		f.stats[testID][k] += n
	}
	return nil
}

func (f *fakeSplitStore) ListSplitStats(_ context.Context, testID string) ([]store.SplitStat, error) {
	var out []store.SplitStat
	for v, n := range f.stats[testID] {
		out = append(out, store.SplitStat{VariantID: v, Bucket: time.Now().Truncate(time.Hour), Requests: n})
	}
	return out, nil
}

func (f *fakeSplitStore) ListDeployURLs(context.Context, string) ([]store.DeployURL, error) {
	return f.urls, nil
}

func (f *fakeSplitStore) ListBranchDeploysByApp(context.Context, string) ([]store.BranchDeploy, error) {
	return f.branches, nil
}

type splitHarness struct {
	svc      *splitService
	st       *fakeSplitStore
	reg      *ingress.Splits
	deploys  []string
	deployEr error
	locked   string
	domains  []string
	nudges   int
	image    string
}

func newSplitHarness(t *testing.T) *splitHarness {
	t.Helper()
	h := &splitHarness{
		st:      &fakeSplitStore{branches: []store.BranchDeploy{{Branch: "redesign", Slug: "redesign", Domain: "web-br-redesign.example.com", DeployAppID: "web-br-redesign"}}},
		reg:     ingress.NewSplits(),
		domains: []string{"web.example.com"},
		image:   "web:2",
	}
	h.svc = &splitService{
		st: h.st, reg: h.reg, defaults: splittest.DefaultsFromEnv(func(string) string { return "" }),
		logger: slog.Default(), now: time.Now,
		appDomains:   func(context.Context, string) ([]string, error) { return h.domains, nil },
		serviceImage: func(context.Context, string) (string, error) { return h.image, nil },
		lockReason: func(context.Context, string) (string, bool) {
			return h.locked, h.locked != ""
		},
		deployImage: func(_ context.Context, app, image string) error {
			if h.deployEr != nil {
				return h.deployEr
			}
			h.deploys = append(h.deploys, app+"="+image)
			return nil
		},
		publicURL: func(_ context.Context, host string) string { return "https://" + host },
		nudge:     func() { h.nudges++ },
	}
	return h
}

func statusOf(err error) int {
	var se splitError
	if errors.As(err, &se) {
		return se.status
	}
	return 0
}

func startReq(weight int) splitStartRequest {
	return splitStartRequest{Variants: []splitVariantInput{{Label: "Redesign", Kind: splittest.KindBranch, Ref: "redesign", Weight: weight}}}
}

func TestSplitStart(t *testing.T) {
	tests := []struct {
		name       string
		req        splitStartRequest
		noDomain   bool
		wantStatus int
	}{
		{"ok adds production leftover", startReq(30), false, 0},
		{"weights over 100", startReq(120), false, http.StatusBadRequest},
		{"unknown branch", splitStartRequest{Variants: []splitVariantInput{{Kind: splittest.KindBranch, Ref: "ghost", Weight: 10}}}, false, http.StatusBadRequest},
		{"bad cookie name", splitStartRequest{Variants: startReq(10).Variants, CookieName: "bad name"}, false, http.StatusBadRequest},
		{"bad cookie ttl", splitStartRequest{Variants: startReq(10).Variants, CookieTTL: "1s"}, false, http.StatusBadRequest},
		{"no domain", startReq(10), true, http.StatusConflict},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newSplitHarness(t)
			if tc.noDomain {
				h.domains = nil
			}
			res, err := h.svc.Start(context.Background(), "web", tc.req)
			if got := statusOf(err); got != tc.wantStatus {
				t.Fatalf("status = %d (err %v), want %d", got, err, tc.wantStatus)
			}
			if tc.wantStatus != 0 {
				if h.reg.Active(h.domains) {
					t.Fatal("a rejected start must not activate ingress")
				}
				return
			}
			if !res.Active || len(res.Variants) != 2 || res.Variants[0].Weight != 70 || res.Variants[0].Kind != splittest.KindProduction {
				t.Fatalf("unexpected resource: %+v", res)
			}
			if !h.reg.Active([]string{"web.example.com"}) || h.nudges == 0 {
				t.Fatal("start must activate ingress and nudge the reconciler")
			}
			if res.Variants[1].QALink == "" || res.Variants[1].Status != "ready" {
				t.Fatalf("variant not ready with QA link: %+v", res.Variants[1])
			}
		})
	}
}

func TestSplitSecondStartConflicts(t *testing.T) {
	h := newSplitHarness(t)
	if _, err := h.svc.Start(context.Background(), "web", startReq(20)); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Start(context.Background(), "web", startReq(20)); statusOf(err) != http.StatusConflict {
		t.Fatalf("second start err = %v, want 409", err)
	}
}

func TestSplitSetWeights(t *testing.T) {
	h := newSplitHarness(t)
	res, err := h.svc.Start(context.Background(), "web", startReq(20))
	if err != nil {
		t.Fatal(err)
	}
	prod, variant := res.Variants[0].ID, res.Variants[1].ID
	tests := []struct {
		name       string
		weights    map[string]int
		wantStatus int
	}{
		{"valid", map[string]int{prod: 40, variant: 60}, 0},
		{"does not sum", map[string]int{prod: 40, variant: 40}, http.StatusBadRequest},
		{"unknown variant", map[string]int{"nope": 100}, http.StatusBadRequest},
		{"by label", map[string]int{"production": 10, "redesign": 90}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.svc.SetWeights(context.Background(), "web", splitWeightsRequest{Weights: tc.weights})
			if got := statusOf(err); got != tc.wantStatus {
				t.Fatalf("status = %d (err %v), want %d", got, err, tc.wantStatus)
			}
		})
	}
}

func TestSplitUnavailableVariantShiftsWeightToProduction(t *testing.T) {
	h := newSplitHarness(t)
	if _, err := h.svc.Start(context.Background(), "web", startReq(40)); err != nil {
		t.Fatal(err)
	}
	h.st.branches = nil
	run, _, _ := h.st.GetRunningSplitTest(context.Background(), "web")
	vs, hosts := h.svc.activeVariants(context.Background(), run)
	if vs[0].Weight != 100 || vs[1].Weight != 0 || len(hosts) != 0 {
		t.Fatalf("variants = %+v hosts = %v", vs, hosts)
	}
	res, _ := h.svc.Get(context.Background(), "web")
	if res.Variants[1].Status != "unavailable" {
		t.Fatalf("status = %q, want unavailable", res.Variants[1].Status)
	}
}

func TestSplitStopAndPromoteHalfSuccess(t *testing.T) {
	ctx := context.Background()
	start := func(t *testing.T) (*splitHarness, string) {
		h := newSplitHarness(t)
		res, err := h.svc.Start(ctx, "web", startReq(30))
		if err != nil {
			t.Fatal(err)
		}
		return h, res.Variants[1].ID
	}

	t.Run("stop clears ingress", func(t *testing.T) {
		h, _ := start(t)
		res, err := h.svc.Stop(ctx, "web", "stopped")
		if err != nil || res.Active || h.reg.Active(h.domains) {
			t.Fatalf("stop: res=%+v err=%v active=%v", res, err, h.reg.Active(h.domains))
		}
		if _, err := h.svc.Stop(ctx, "web", "stopped"); statusOf(err) != http.StatusNotFound {
			t.Fatalf("second stop err = %v, want 404", err)
		}
	})
	t.Run("stop store failure keeps split running", func(t *testing.T) {
		h, _ := start(t)
		h.st.endErr = errors.New("disk full")
		if _, err := h.svc.Stop(ctx, "web", "stopped"); err == nil {
			t.Fatal("want error")
		}
		if !h.reg.Active(h.domains) {
			t.Fatal("a failed stop must leave the split serving")
		}
	})
	t.Run("promote deploys then ends", func(t *testing.T) {
		h, variant := start(t)
		res, err := h.svc.Promote(ctx, "web", variant)
		if err != nil || res.Active || len(h.deploys) != 1 || h.deploys[0] != "web=web:2" || h.reg.Active(h.domains) {
			t.Fatalf("promote: res=%+v err=%v deploys=%v", res, err, h.deploys)
		}
	})
	t.Run("deploy failure leaves test running", func(t *testing.T) {
		h, variant := start(t)
		h.deployEr = errors.New("registry down")
		_, err := h.svc.Promote(ctx, "web", variant)
		if statusOf(err) != http.StatusBadGateway {
			t.Fatalf("err = %v, want 502", err)
		}
		if _, ok, _ := h.st.GetRunningSplitTest(ctx, "web"); !ok || !h.reg.Active(h.domains) {
			t.Fatal("test must still be running after a failed deploy")
		}
	})
	t.Run("end failure after deploy is retryable", func(t *testing.T) {
		h, variant := start(t)
		h.st.endErr = errors.New("db locked")
		if _, err := h.svc.Promote(ctx, "web", variant); statusOf(err) != http.StatusBadGateway {
			t.Fatalf("err = %v, want 502", err)
		}
		if len(h.deploys) != 1 {
			t.Fatalf("deploys = %v, want one", h.deploys)
		}
		h.st.endErr = nil
		if res, err := h.svc.Promote(ctx, "web", variant); err != nil || res.Active {
			t.Fatalf("retry: res=%+v err=%v", res, err)
		}
	})
	t.Run("publish lock refuses promote", func(t *testing.T) {
		h, variant := start(t)
		h.locked = "release freeze"
		if _, err := h.svc.Promote(ctx, "web", variant); statusOf(err) != http.StatusConflict {
			t.Fatalf("err = %v, want 409", err)
		}
		if len(h.deploys) != 0 {
			t.Fatal("a locked app must not deploy")
		}
	})
	t.Run("production cannot be promoted", func(t *testing.T) {
		h, _ := start(t)
		res, _ := h.svc.Get(ctx, "web")
		if _, err := h.svc.Promote(ctx, "web", res.Variants[0].ID); statusOf(err) != http.StatusBadRequest {
			t.Fatalf("err = %v, want 400", err)
		}
	})
}

func TestSplitStatsFlushedIntoResource(t *testing.T) {
	h := newSplitHarness(t)
	res, err := h.svc.Start(context.Background(), "web", startReq(50))
	if err != nil {
		t.Fatal(err)
	}
	h.st.stats = map[string]map[string]uint64{res.TestID: {res.Variants[0].ID: 30, res.Variants[1].ID: 10}}
	got, err := h.svc.Get(context.Background(), "web")
	if err != nil || got.TotalRequests != 40 || got.Variants[1].Requests != 10 || got.Variants[1].Share != 0.25 {
		t.Fatalf("stats resource: %+v err=%v", got, err)
	}
	if len(got.Hourly) == 0 {
		t.Fatal("expected hourly buckets")
	}
}
