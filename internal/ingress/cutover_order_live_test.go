package ingress

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"syscall"
	"testing"
	"time"
)

// cutoverRig is a live Caddy with one app route whose backend a test moves
// between releases while a request loop runs, the way the reconciler does.
type cutoverRig struct {
	t    *testing.T
	d    *Driver
	addr string
	h    Hardening
}

func newCutoverRig(t *testing.T) *cutoverRig {
	t.Helper()
	r := &cutoverRig{t: t, d: New(testLogger(t)), addr: fmt.Sprintf("127.0.0.1:%d", freePort(t)), h: DefaultHardening()}
	t.Cleanup(func() { _ = r.d.Stop(context.Background()) })
	return r
}

func (r *cutoverRig) route(dial string) {
	r.t.Helper()
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "cutover", ListenAddr: r.addr, StorageDir: r.t.TempDir(), Hardening: &r.h,
		Routes: []ProxyRoute{{Hosts: []string{"127.0.0.1"}, BackendDial: dial}},
	})
	if err != nil {
		r.t.Fatal(err)
	}
	cfg.Apps.HTTP.Servers["cutover"].AutomaticHTTPS = &AutoHTTPSConfig{Disabled: true}
	if err := r.d.Apply(context.Background(), cfg); err != nil {
		r.t.Fatalf("Apply() error: %v", err)
	}
}

// release is one app container: a backend that can be stopped for real,
// refusing new dials and cutting open connections like a SIGKILLed process.
type release struct{ *lbReplica }

func newRelease(t *testing.T, name string) release {
	t.Helper()
	return release{newLBReplica(t, name)}
}

func (r release) dial() string { return r.Listener.Addr().String() }

func (r release) stop() {
	r.Config.SetKeepAlivesEnabled(false)
	_ = r.Listener.Close()
	r.CloseClientConnections()
}

// requestLoop mimics a curl loop: a fresh connection per request, a short
// client timeout, and every answer other than a 200 counted.
type requestLoop struct {
	mu       sync.Mutex
	ok       map[string]int
	failures []string
	resets   int
	stop     chan struct{}
	wg       sync.WaitGroup
}

// A connection reset or EOF racing a config reload is counted apart from
// failures: it comes from the reload itself, not from the cutover order.
func startRequestLoop(addr string) *requestLoop {
	l := &requestLoop{ok: map[string]int{}, stop: make(chan struct{})}
	c := &http.Client{Timeout: time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	for range 4 {
		l.wg.Add(1)
		go l.run(c, "http://"+addr+"/")
	}
	return l
}

func (l *requestLoop) run(c *http.Client, url string) {
	defer l.wg.Done()
	for {
		select {
		case <-l.stop:
			return
		default:
		}
		resp, err := c.Get(url) //nolint:noctx // test loop with a client timeout
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNRESET) {
				l.mu.Lock()
				l.resets++
				l.mu.Unlock()
				continue
			}
			l.fail(err.Error())
			continue
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			l.fail(fmt.Sprintf("status %d", resp.StatusCode))
		} else {
			l.mu.Lock()
			l.ok[string(b)]++
			l.mu.Unlock()
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (l *requestLoop) fail(why string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures = append(l.failures, time.Now().Format("15:04:05.000")+" "+why)
}

func (l *requestLoop) finish() (map[string]int, []string) {
	close(l.stop)
	l.wg.Wait()
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.ok, l.failures
}

func waitListening(t *testing.T, addr string) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if c, err := net.Dial("tcp", addr); err == nil {
			_ = c.Close()
			return
		}
	}
	t.Fatalf("listener %s never came up", addr)
}

// The order the reconciler must keep on deploy, restart and rollback: the
// route moves to a ready release first and the old one stops only after a
// drain. Under that order a request loop sees no answer other than a 200.
func TestCutoverOrder_ZeroNon200(t *testing.T) {
	const drain = 300 * time.Millisecond
	rig := newCutoverRig(t)
	v1 := newRelease(t, "v1")
	rig.route(v1.dial())
	waitListening(t, rig.addr)
	loop := startRequestLoop(rig.addr)
	settle := func() { time.Sleep(400 * time.Millisecond) }
	settle()

	// Deploy: v2 is ready, the route moves, v1 is held for a rollback.
	v2 := newRelease(t, "v2")
	rig.route(v2.dial())
	settle()

	// Restart: a fresh v2 container replaces the old one after its drain.
	v2b := newRelease(t, "v2")
	rig.route(v2b.dial())
	time.Sleep(drain)
	v2.stop()
	settle()

	// Rollback onto the held v1, then the newer release stops after drain.
	rig.route(v1.dial())
	time.Sleep(drain)
	v2b.stop()
	settle()

	ok, failures := loop.finish()
	if len(failures) > 0 {
		t.Fatalf("%d non-200 answers (ok %v), first: %v", len(failures), ok, failures[:min(len(failures), 5)])
	}
	if loop.resets > 0 {
		t.Logf("%d requests reset by a config reload, not by the cutover order", loop.resets)
	}
	for _, v := range []string{"v1", "v2"} {
		if ok[v] == 0 {
			t.Errorf("no request reached %s (ok %v), the cutover was not exercised", v, ok)
		}
	}
}

// Stopping the serving release before the route moves is what turns a
// rollback into dropped requests: in-flight and new requests retry a dead
// dial until the route flips, past a short client timeout.
func TestCutoverOrder_StopBeforeRouteDrops(t *testing.T) {
	rig := newCutoverRig(t)
	v1, v2 := newRelease(t, "v1"), newRelease(t, "v2")
	rig.route(v2.dial())
	waitListening(t, rig.addr)
	loop := startRequestLoop(rig.addr)
	time.Sleep(300 * time.Millisecond)

	v2.stop()
	time.Sleep(time.Second)
	rig.route(v1.dial())
	time.Sleep(2500 * time.Millisecond)

	ok, failures := loop.finish()
	if len(failures) == 0 {
		t.Fatalf("no request failed with the serving release stopped before the route moved (ok %v)", ok)
	}
	if ok["v1"] == 0 {
		t.Errorf("the route never recovered onto v1 (ok %v)", ok)
	}
}

// Moving the route onto a release that runs but does not listen yet (a held
// container just restarted by a rollback) drops requests for its startup time.
func TestCutoverOrder_RouteBeforeReadyDrops(t *testing.T) {
	rig := newCutoverRig(t)
	v2 := newRelease(t, "v2")
	rig.route(v2.dial())
	waitListening(t, rig.addr)
	loop := startRequestLoop(rig.addr)
	time.Sleep(300 * time.Millisecond)

	starting := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	rig.route(starting)
	time.Sleep(1500 * time.Millisecond)
	ln, err := net.Listen("tcp", starting)
	if err != nil {
		t.Skipf("port %s was taken meanwhile: %v", starting, err)
	}
	srv := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "v1") })}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	time.Sleep(time.Second)

	ok, failures := loop.finish()
	if len(failures) == 0 {
		t.Fatalf("no request failed while the route pointed at a release still starting (ok %v)", ok)
	}
	if ok["v1"] == 0 {
		t.Errorf("the started release never served (ok %v)", ok)
	}
}
