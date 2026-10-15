package ingress

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestRouteCheck_ConfigFirstAndValidated(t *testing.T) {
	rc, err := NewRouteCheck()
	if err != nil || !rc.Valid() {
		t.Fatalf("NewRouteCheck() = %+v, %v", rc, err)
	}
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "s", ListenAddr: ":8443", RouteCheck: &rc,
		Routes:            []ProxyRoute{{Hosts: []string{"*.example.com"}, BackendDial: "127.0.0.1:1"}},
		MaintenanceRoutes: []MaintenanceRoute{{Hosts: []string{"down.example.com"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	first := cfg.Apps.HTTP.Servers["s"].Routes[0]
	if got := first.Match[0].Header[RouteCheckHeader]; len(got) != 1 || got[0] != rc.Token || len(first.Match[0].Host) != 0 {
		t.Fatalf("first route = %+v, want the route check on every host", first)
	}
	reply := first.Handle[0].(StaticResponseHandler)
	if reply.StatusCode != http.StatusNoContent || reply.Headers[RouteCheckReplyHeader][0] != rc.Reply {
		t.Errorf("reply = %+v", reply)
	}
	for _, bad := range []RouteCheck{{}, {Token: rc.Token, Reply: rc.Token}, {Token: "x", Reply: rc.Reply}} {
		if _, err := BuildRoutesConfig(RoutesOptions{ServerName: "s", ListenAddr: ":8443", RouteCheck: &bad}); err == nil {
			t.Errorf("RouteCheck %+v accepted", bad)
		}
	}
}

// TestRouteCheck_Live_AnsweredOnProxiedListener: the marker answers on the
// plain listener a proxy forwards to, and normal requests still reach the app.
func TestRouteCheck_Live_AnsweredOnProxiedListener(t *testing.T) {
	skipUnderLoad(t)
	hits := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte("real app"))
	}))
	defer backend.Close()
	rc, err := NewRouteCheck()
	if err != nil {
		t.Fatal(err)
	}
	storage, err := os.MkdirTemp("", "route-check-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(storage) })
	httpPort := freePort(t)
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "check", ListenAddr: fmt.Sprintf("127.0.0.1:%d", freePort(t)), HTTPPort: httpPort,
		PlainHTTP: true, TLS: true, StorageDir: storage, RouteCheck: &rc,
		Routes:            []ProxyRoute{{Hosts: []string{"app.test"}, BackendDial: backend.Listener.Addr().String()}},
		MaintenanceRoutes: []MaintenanceRoute{{Hosts: []string{"down.test"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	d := New(testLogger(t))
	if err := d.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("apply: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop(context.Background()) })
	base := fmt.Sprintf("http://127.0.0.1:%d", httpPort)

	for _, host := range []string{"app.test", "down.test"} {
		resp, _ := doReq(t, "GET", base, host, "", map[string]string{RouteCheckHeader: rc.Token})
		if resp.StatusCode != http.StatusNoContent || resp.Header.Get(RouteCheckReplyHeader) != rc.Reply {
			t.Errorf("%s marker: status=%d reply=%q", host, resp.StatusCode, resp.Header.Get(RouteCheckReplyHeader))
		}
	}
	resp, _ := doReq(t, "GET", base, "app.test", "", map[string]string{RouteCheckHeader: "wrong"})
	if resp.Header.Get(RouteCheckReplyHeader) != "" {
		t.Error("a wrong token got the reply")
	}
	resp, body := doReq(t, "GET", base, "app.test", "", nil)
	if resp.StatusCode != http.StatusOK || body != "real app" {
		t.Errorf("plain request: status=%d body=%q", resp.StatusCode, body)
	}
	if hits != 2 {
		t.Errorf("backend hits = %d, want 2 (wrong token and plain request)", hits)
	}
}
