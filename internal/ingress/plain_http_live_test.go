package ingress

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestPlainHTTP_Live proves a proxy that terminates TLS can reach the routes
// over plain HTTP while the TLS listener keeps serving.
func TestPlainHTTP_Live(t *testing.T) {
	skipUnderLoad(t)
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "app ok")
	}))
	defer app.Close()

	tlsPort, httpPort := freePort(t), freePort(t)
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "plain",
		ListenAddr: fmt.Sprintf("127.0.0.1:%d", tlsPort),
		HTTPPort:   httpPort,
		PlainHTTP:  true,
		TLS:        true,
		Routes:     []ProxyRoute{{Hosts: []string{"app.test"}, BackendDial: app.Listener.Addr().String()}},
		StorageDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	d := New(testLogger(t))
	if err := d.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("apply: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop(context.Background()) })

	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/", httpPort), nil)
	req.Host = "app.test"
	resp, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if err != nil {
		t.Fatalf("plain request: %v", err)
	}
	body := readAll(resp)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || body != "app ok" {
		t.Errorf("plain = %d %q, want 200 \"app ok\"", resp.StatusCode, body)
	}

	tc := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{ServerName: "app.test", InsecureSkipVerify: true}}} //nolint:gosec // local internal CA
	var sresp *http.Response
	deadline := time.Now().Add(10 * time.Second)
	for {
		sreq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("https://127.0.0.1:%d/", tlsPort), nil)
		sreq.Host = "app.test"
		sresp, err = tc.Do(sreq)
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("tls request: %v", err)
	}
	sbody := readAll(sresp)
	_ = sresp.Body.Close()
	if sresp.StatusCode != http.StatusOK || sbody != "app ok" {
		t.Errorf("tls = %d %q, want 200 \"app ok\"", sresp.StatusCode, sbody)
	}
}
