package ingress

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewLBReverseProxyHandler(t *testing.T) {
	tests := []struct {
		name string
		lb   LBRoute
		want string
	}{
		{
			name: "round robin default has no load_balancing block",
			lb:   LBRoute{Upstreams: []string{"127.0.0.1:1", "127.0.0.1:2"}},
			want: `{"handler":"reverse_proxy","upstreams":[{"dial":"127.0.0.1:1"},{"dial":"127.0.0.1:2"}]}`,
		},
		{
			name: "least conn with retries",
			lb:   LBRoute{Upstreams: []string{"a:1"}, Policy: LBPolicyLeastConn, Retries: 2, TryDuration: "5s"},
			want: `{"handler":"reverse_proxy","upstreams":[{"dial":"a:1"}],"load_balancing":{"selection_policy":{"policy":"least_conn"},"retries":2,"try_duration":"5s"}}`,
		},
		{
			name: "weighted",
			lb:   LBRoute{Upstreams: []string{"a:1", "b:1"}, Policy: LBPolicyWeighted, Weights: []int{3, 1}},
			want: `{"handler":"reverse_proxy","upstreams":[{"dial":"a:1"},{"dial":"b:1"}],"load_balancing":{"selection_policy":{"policy":"weighted_round_robin","weights":[3,1]}}}`,
		},
		{
			name: "cookie sticky",
			lb:   LBRoute{Upstreams: []string{"a:1"}, Policy: LBPolicyCookie, CookieName: "sid"},
			want: `{"handler":"reverse_proxy","upstreams":[{"dial":"a:1"}],"load_balancing":{"selection_policy":{"policy":"cookie","name":"sid","fallback":{"policy":"round_robin"}}}}`,
		},
		{
			name: "health, transport and drain",
			lb: LBRoute{
				Upstreams:             []string{"a:1"},
				ActiveHealth:          &ActiveHealthChecks{URI: "/healthz", Interval: "5s", Timeout: "2s", Passes: 2, Fails: 3},
				PassiveHealth:         &PassiveHealthChecks{FailDuration: "30s", MaxFails: 3},
				UpstreamTLS:           &UpstreamTLSConfig{InsecureSkipVerify: true},
				ResponseHeaderTimeout: "10s",
				StreamCloseDelay:      "15s",
			},
			want: `{"handler":"reverse_proxy","upstreams":[{"dial":"a:1"}],"health_checks":{"active":{"uri":"/healthz","interval":"5s","timeout":"2s","passes":2,"fails":3},"passive":{"fail_duration":"30s","max_fails":3}},"transport":{"protocol":"http","tls":{"insecure_skip_verify":true},"response_header_timeout":"10s"},"stream_close_delay":"15s"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(NewLBReverseProxyHandler(&tt.lb))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("handler json\n got: %s\nwant: %s", got, tt.want)
			}
		})
	}
}

func TestBuildRoutesConfig_LBRoute(t *testing.T) {
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "ingress", ListenAddr: ":8443",
		Routes: []ProxyRoute{{
			Hosts: []string{"app.example.internal"},
			LB:    &LBRoute{Upstreams: []string{"a:1", "b:1"}, RateLimitRPS: 5},
		}},
	})
	if err != nil {
		t.Fatalf("BuildRoutesConfig() error: %v", err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"handler":"rate_limit"`, `"lb-app.example.internal-sustained"`, `{"dial":"b:1"}`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("config missing %s: %s", want, raw)
		}
	}

	if _, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "ingress", ListenAddr: ":8443",
		Routes: []ProxyRoute{{Hosts: []string{"x.example.internal"}, LB: &LBRoute{}}},
	}); err == nil || !strings.Contains(err.Error(), "no backend dial address") {
		t.Errorf("empty LB pool error = %v, want no backend dial address", err)
	}
}

func TestDriver_LoadBalancer_RoundRobinAndFailover(t *testing.T) {
	skipUnderLoad(t)
	newBackend := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" {
				w.WriteHeader(http.StatusOK)
				return
			}
			_, _ = fmt.Fprint(w, name)
		}))
	}
	a, b := newBackend("a"), newBackend("b")
	defer a.Close()
	defer b.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "lb-test", ListenAddr: addr, StorageDir: t.TempDir(),
		Routes: []ProxyRoute{{
			Hosts: []string{"127.0.0.1"},
			LB: &LBRoute{
				Upstreams:     []string{a.Listener.Addr().String(), b.Listener.Addr().String()},
				Policy:        LBPolicyRoundRobin,
				Retries:       2,
				TryDuration:   "3s",
				ActiveHealth:  &ActiveHealthChecks{URI: "/healthz", Interval: "1s", Timeout: "1s"},
				PassiveHealth: &PassiveHealthChecks{FailDuration: "5s", MaxFails: 1},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Apps.HTTP.Servers["lb-test"].AutomaticHTTPS = &AutoHTTPSConfig{Disabled: true}

	d := New(testLogger(t))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := d.Apply(ctx, cfg); err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	defer func() { _ = d.Stop(ctx) }()

	seen := map[string]bool{}
	for i := 0; i < 6; i++ {
		seen[getBodyWithRetry(t, "http://"+addr+"/")] = true
	}
	if !seen["a"] || !seen["b"] {
		t.Fatalf("round robin served %v, want both backends", seen)
	}

	a.Close()
	for i := 0; i < 4; i++ {
		resp, err := http.Get("http://" + addr + "/")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK || string(body) != "b" {
			t.Fatalf("after backend a died: status=%d body=%q, want 200 from b", resp.StatusCode, body)
		}
	}
}
