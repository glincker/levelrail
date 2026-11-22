package preview

import (
	"context"
	"net/netip"
	"sync"
	"testing"
	"time"
)

// gateResolver wraps fakeResolver with a RenderGate and a per-call target.
type gateResolver struct {
	*fakeResolver
	allow bool
}

func (g gateResolver) Renderable(context.Context, string) (bool, error) { return g.allow, nil }

func shotHarness(t *testing.T, mutate func(*Config)) *harness {
	t.Helper()
	return newHarness(t, func(c *Config) {
		c.DefaultMode = ModeScreenshot
		if mutate != nil {
			mutate(c)
		}
	})
}

func (h *harness) states(t *testing.T, apps ...string) map[string]CardState {
	t.Helper()
	st, err := h.m.CardStates(context.Background(), apps)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestCardStates_QueuesOneRenderAndReachesReady(t *testing.T) {
	h := shotHarness(t, nil)
	h.shoot.block = make(chan struct{})
	if st := h.states(t, "web")["web"]; st.State != StateQueued && st.State != StateRendering {
		t.Fatalf("first view state = %q, want queued or rendering", st.State)
	}
	// A second list view while it renders must not queue another job.
	for i := 0; i < 3; i++ {
		h.states(t, "web")
	}
	close(h.shoot.block)
	h.waitIdle(t, "web")
	if h.shoot.calls != 1 {
		t.Errorf("shots = %d, want exactly 1", h.shoot.calls)
	}
	st := h.states(t, "web")["web"]
	if st.State != StateReady || st.Record == nil || !st.Record.HasFile() {
		t.Errorf("state after render = %+v, want ready with a file", st)
	}
}

func TestCardStates_FailedIsStickyUntilNextDeploy(t *testing.T) {
	h := shotHarness(t, nil)
	h.shoot.result = &ShotResult{HTTPStatus: 500, FinalURL: "http://web:3000/"}
	h.states(t, "web")
	h.waitIdle(t, "web")
	st := h.states(t, "web")["web"]
	if st.State != StateFailed || st.Reason == "" {
		t.Fatalf("state = %+v, want failed with a reason", st)
	}
	h.states(t, "web")
	h.waitIdle(t, "web")
	if h.shoot.calls != 1 {
		t.Errorf("shots = %d, a failure must not retry on view", h.shoot.calls)
	}
	h.shoot.result = &ShotResult{HTTPStatus: 200, FinalURL: "http://web:3000/", PNG: stripedPNG(t)}
	h.res.mu.Lock()
	h.res.target.DeploymentID = "dep_2"
	h.res.mu.Unlock()
	h.m.NotifyReady("web", "img:2", "id-2")
	h.waitIdle(t, "web")
	if st := h.states(t, "web")["web"]; st.State != StateReady {
		t.Errorf("after redeploy state = %+v, want ready", st)
	}
}

func TestRender_MinIntervalSwallowsRedeployStorm(t *testing.T) {
	h := shotHarness(t, func(c *Config) { c.MinInterval = time.Hour })
	h.m.NotifyReady("web", "img:1", "id-1")
	h.waitIdle(t, "web")
	h.res.mu.Lock()
	h.res.target.DeploymentID = "dep_2"
	h.res.mu.Unlock()
	h.m.NotifyReady("web", "img:2", "id-2")
	h.waitIdle(t, "web")
	if h.shoot.calls != 1 {
		t.Fatalf("shots = %d inside the interval, want 1", h.shoot.calls)
	}
	h.advance(2 * time.Hour)
	// The swallowed deploy renders on the next view once the interval passed.
	h.states(t, "web")
	h.waitIdle(t, "web")
	if h.shoot.calls != 2 {
		t.Errorf("shots = %d after the interval, want 2", h.shoot.calls)
	}
	if _, ok := h.record("dep_2"); !ok {
		t.Error("the redeployed release has no record")
	}
}

func TestRender_KindGateSkipsNonWebApps(t *testing.T) {
	h := shotHarness(t, nil)
	h.m.deps.Resolver = gateResolver{fakeResolver: h.res, allow: false}
	h.m.NotifyReady("worker", "img:1", "id-1")
	h.waitIdle(t, "worker")
	if h.shoot.calls != 0 || h.run.started != 0 || len(h.store.records) != 0 {
		t.Errorf("shots=%d started=%d records=%d, want none for a gated kind", h.shoot.calls, h.run.started, len(h.store.records))
	}
	// A manual capture is the owner asking, so the gate does not apply.
	h.m.deps.Resolver = gateResolver{fakeResolver: h.res, allow: false}
	if err := h.m.Capture(context.Background(), "worker"); err != nil {
		t.Fatal(err)
	}
	h.waitIdle(t, "worker")
	if h.shoot.calls != 1 {
		t.Errorf("manual shots = %d, want 1", h.shoot.calls)
	}
}

func TestRender_MaxConcurrentBoundsWorkers(t *testing.T) {
	for _, tc := range []struct{ conc, want int }{{1, 1}, {2, 2}} {
		h := shotHarness(t, func(c *Config) { c.MaxConcurrent = tc.conc })
		h.shoot.block = make(chan struct{})
		for _, app := range []string{"a", "b", "c"} {
			h.m.enqueue(job{app: app, force: true})
		}
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			h.shoot.mu.Lock()
			n := h.shoot.active
			h.shoot.mu.Unlock()
			if n >= tc.want {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		time.Sleep(50 * time.Millisecond)
		close(h.shoot.block)
		for _, app := range []string{"a", "b", "c"} {
			h.waitIdle(t, app)
		}
		if h.shoot.maxAct != tc.want {
			t.Errorf("max concurrency = %d with limit %d", h.shoot.maxAct, tc.conc)
		}
	}
}

func TestRender_DedupesQueuedApp(t *testing.T) {
	h := shotHarness(t, func(c *Config) { c.QueueDepth = 8 })
	h.m.mu.Lock()
	h.m.workers = 1
	h.m.mu.Unlock()
	for i := 0; i < 3; i++ {
		h.m.enqueue(job{app: "a", auto: true})
	}
	h.m.mu.Lock()
	defer h.m.mu.Unlock()
	if len(h.m.queue) != 1 {
		t.Errorf("queue = %d jobs for one app, want 1", len(h.m.queue))
	}
}

func TestRender_IdleStopKeepsBrowserWarmThenStopsIt(t *testing.T) {
	h := shotHarness(t, func(c *Config) { c.IdleStop = 150 * time.Millisecond })
	h.m.enqueue(job{app: "a", force: true})
	h.waitIdle(t, "a")
	h.res.mu.Lock()
	h.res.target.DeploymentID = "dep_2"
	h.res.mu.Unlock()
	h.m.enqueue(job{app: "a", force: true})
	h.waitIdle(t, "a")
	h.run.mu.Lock()
	started, closed := h.run.started, h.run.closed
	h.run.mu.Unlock()
	if started != 1 || closed != 0 {
		t.Fatalf("started=%d closed=%d, want one warm browser reused", started, closed)
	}
	deadline := time.Now().Add(3 * time.Second)
	for h.m.pool.count() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	h.run.mu.Lock()
	defer h.run.mu.Unlock()
	if h.m.pool.count() != 0 || h.run.closed != 1 {
		t.Errorf("pool=%d closed=%d, want the idle browser stopped", h.m.pool.count(), h.run.closed)
	}
}

func TestRender_RemoteNodeWithoutPublicURLFailsWithReason(t *testing.T) {
	h := shotHarness(t, nil)
	h.res.err = &SkipError{DeploymentID: "dep_1", Reason: ReasonRemoteNode, Detail: "no public address"}
	h.states(t, "web")
	h.waitIdle(t, "web")
	st := h.states(t, "web")["web"]
	if st.State != StateFailed || st.Reason != ReasonRemoteNode {
		t.Errorf("state = %+v, want failed remote_node", st)
	}
	if h.run.started != 0 {
		t.Error("a remote app without a public address must not start a browser")
	}
}

func TestRender_PublicURLIsPinnedAndInternalRefused(t *testing.T) {
	h := shotHarness(t, nil)
	var mu sync.Mutex
	answer := []netip.Addr{netip.MustParseAddr("93.184.216.34")}
	h.m.lookup = func(context.Context, string) ([]netip.Addr, error) {
		mu.Lock()
		defer mu.Unlock()
		return answer, nil
	}
	h.res.target = Target{DeploymentID: "dep_1", Image: "img:1", PublicURL: "https://shop.example.com"}
	h.shoot.result = &ShotResult{HTTPStatus: 200, FinalURL: "https://shop.example.com/", PNG: stripedPNG(t)}
	h.m.enqueue(job{app: "web", force: true})
	h.waitIdle(t, "web")
	if h.shoot.calls != 1 || h.shoot.reqs[0].URL != "https://shop.example.com/" {
		t.Fatalf("calls=%d reqs=%+v", h.shoot.calls, h.shoot.reqs)
	}
	last := h.run.specs[len(h.run.specs)-1]
	if last.Network != publicNetwork || last.Command[len(last.Command)-1] != "--host-resolver-rules=MAP shop.example.com 93.184.216.34" {
		t.Errorf("spec = %+v", last)
	}

	mu.Lock()
	answer = []netip.Addr{netip.MustParseAddr("10.0.0.5")}
	mu.Unlock()
	h.res.target.DeploymentID = "dep_2"
	h.m.enqueue(job{app: "web", force: true})
	h.waitIdle(t, "web")
	if r, ok := h.record("dep_2"); !ok || r.Reason != ReasonBlockedTarget || h.shoot.calls != 1 {
		t.Errorf("record=%+v ok=%v shots=%d, want blocked_target and no shot", r, ok, h.shoot.calls)
	}
}

func TestLive_SourceOverridesDefaultMode(t *testing.T) {
	h := shotHarness(t, nil)
	if got := h.states(t, "web")["web"]; got.State == StateNone {
		t.Fatal("screenshot default must queue a render")
	}
	h.waitIdle(t, "web")
	h2 := shotHarness(t, nil)
	h2.m.liveSrc = func(context.Context) Live { return Live{DefaultMode: ModeMetadata, MaxConcurrent: 1} }
	if got := h2.states(t, "web")["web"]; got.State != StateNone {
		t.Errorf("metadata default state = %q, want none and no render", got.State)
	}
	h2.waitIdle(t, "web")
	if h2.shoot.calls != 0 {
		t.Error("metadata default must not render")
	}
}
