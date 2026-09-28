package models

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// wakeStore is a gateway store whose model row can change under it, like
// the reconciler changing it in production.
type wakeStoreFake struct {
	mu      sync.Mutex
	model   store.Model
	touches []time.Time
}

func (w *wakeStoreFake) ListModels(context.Context) ([]store.Model, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return []store.Model{w.model}, nil
}

func (w *wakeStoreFake) ListActiveModelKeys(context.Context) ([]store.ModelKey, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return []store.ModelKey{{ID: store.DefaultModelKeyID(w.model.Name), ModelName: w.model.Name, Name: store.DefaultModelKeyName, KeyHash: w.model.APIKeyHash}}, nil
}

func (w *wakeStoreFake) GetModel(context.Context, string) (*store.Model, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	cp := w.model
	return &cp, nil
}

func (w *wakeStoreFake) TouchModel(_ context.Context, _ string, at time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.touches = append(w.touches, at)
	w.model.LastActiveAt = at
	return nil
}

func (w *wakeStoreFake) touchCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.touches)
}

func (w *wakeStoreFake) wakeUp(dial string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.model.ResidencyState = store.ResidencyAwake
	w.model.EndpointDial = dial
}

type wakeRig struct {
	gw    *Gateway
	st    *wakeStoreFake
	key   string
	dial  string
	nudge *atomic.Int32
}

func newWakeRig(t *testing.T, wait time.Duration, state string) wakeRig {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(upstream.Close)
	key, hash, _, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	st := &wakeStoreFake{model: store.Model{Name: "chat", Domain: "chat.example.com", APIKeyHash: hash,
		Residency: store.ResidencyOnDemand, ResidencyState: state, EndpointDial: strings.TrimPrefix(upstream.URL, "http://")}}
	gw := NewGateway(st, NewHostResolver("1-2-3-4", fallbackFn), slog.New(slog.NewTextHandler(io.Discard, nil)))
	l := DefaultGatewayLimits()
	l.WakeWait, l.WakeRetryAfter, l.TouchInterval = wait, 7*time.Second, time.Hour
	gw.SetLimits(l)
	nudges := &atomic.Int32{}
	gw.SetWakeHook(func() { nudges.Add(1) })
	return wakeRig{gw: gw, st: st, key: key, dial: st.model.EndpointDial, nudge: nudges}
}

func (r wakeRig) do(ctx context.Context, auth string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil).WithContext(ctx)
	req.Host = "chat.example.com"
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+auth)
	}
	rec := httptest.NewRecorder()
	r.gw.Handle(rec, req)
	return rec
}

func TestGatewayWake_HoldsRequestUntilEngineServes(t *testing.T) {
	rig := newWakeRig(t, 5*time.Second, store.ResidencyAsleep)
	go func() {
		time.Sleep(900 * time.Millisecond)
		rig.st.wakeUp(rig.dial)
	}()
	start := time.Now()
	rec := rig.do(context.Background(), rig.key)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if time.Since(start) < 800*time.Millisecond {
		t.Fatal("the request must have waited for the wake")
	}
	if rig.st.touchCount() == 0 || rig.nudge.Load() == 0 {
		t.Fatalf("a wake must touch the model and nudge the reconciler: touches=%d nudges=%d", rig.st.touchCount(), rig.nudge.Load())
	}
}

func TestGatewayWake_TimesOutWith503AndRetryAfter(t *testing.T) {
	rig := newWakeRig(t, 700*time.Millisecond, store.ResidencyAsleep)
	rec := rig.do(context.Background(), rig.key)
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") != "7" {
		t.Fatalf("status = %d retry-after = %q body = %s", rec.Code, rec.Header().Get("Retry-After"), rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "model_waking") {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if rig.st.touchCount() == 0 {
		t.Fatal("even a timed out request must have asked for the wake, so the next retry finds it loading")
	}
}

func TestGatewayWake_ZeroWaitFailsFastButStillWakes(t *testing.T) {
	rig := newWakeRig(t, 0, store.ResidencyAsleep)
	start := time.Now()
	rec := rig.do(context.Background(), rig.key)
	if rec.Code != http.StatusServiceUnavailable || time.Since(start) > 400*time.Millisecond {
		t.Fatalf("status = %d after %s", rec.Code, time.Since(start))
	}
	if rig.st.touchCount() != 1 || rig.nudge.Load() == 0 {
		t.Fatalf("touches = %d nudges = %d", rig.st.touchCount(), rig.nudge.Load())
	}
}

func TestGatewayWake_UnauthenticatedRequestNeverWakes(t *testing.T) {
	rig := newWakeRig(t, 200*time.Millisecond, store.ResidencyAsleep)
	for _, auth := range []string{"", "lr-wrong"} {
		if rec := rig.do(context.Background(), auth); rec.Code != http.StatusUnauthorized {
			t.Fatalf("auth %q status = %d", auth, rec.Code)
		}
	}
	if rig.st.touchCount() != 0 || rig.nudge.Load() != 0 {
		t.Fatalf("touches = %d nudges = %d, want none", rig.st.touchCount(), rig.nudge.Load())
	}
}

func TestGatewayWake_ConcurrentRequestsShareOneWake(t *testing.T) {
	rig := newWakeRig(t, 5*time.Second, store.ResidencyAsleep)
	go func() {
		time.Sleep(700 * time.Millisecond)
		rig.st.wakeUp(rig.dial)
	}()
	var wg sync.WaitGroup
	codes := make([]int, 6)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = rig.do(context.Background(), rig.key).Code
		}()
	}
	wg.Wait()
	for i, c := range codes {
		if c != http.StatusOK {
			t.Fatalf("request %d status = %d", i, c)
		}
	}
	if n := rig.st.touchCount(); n > 2 {
		t.Fatalf("touch writes = %d, want them throttled to about one", n)
	}
}

func TestGatewayWake_ClientLeavingEndsTheWaitWithoutAResponse(t *testing.T) {
	rig := newWakeRig(t, 10*time.Second, store.ResidencyAsleep)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	rec := rig.do(ctx, rig.key)
	if time.Since(start) > 3*time.Second {
		t.Fatal("the wait must end when the client leaves")
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("nothing should be written to a gone client: %s", rec.Body.String())
	}
}

func TestGatewayWake_AwakeModelIsProxiedAndKeptActive(t *testing.T) {
	rig := newWakeRig(t, time.Second, store.ResidencyAwake)
	rec := rig.do(context.Background(), rig.key)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rig.st.touchCount() == 0 {
		t.Fatal("traffic must keep an on-demand model from idling")
	}
	if rig.nudge.Load() != 0 {
		t.Fatal("an awake model needs no wake")
	}
}
