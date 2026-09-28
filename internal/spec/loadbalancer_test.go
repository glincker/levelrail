package spec

import (
	"strings"
	"testing"
)

func TestParse_LoadBalancer(t *testing.T) {
	const doc = `
version: 1
services:
  web:
    build: { type: dockerfile }
    port: 8080
    replicas: 3
    loadbalancer:
      algorithm: weighted
      weights: [3, 2, 1]
      active_health: { path: /healthz, interval: 5s, timeout: 2s, passes: 2, fails: 3 }
      passive_health: { fail_duration: 30s, max_fails: 3 }
      retries: { count: 2, try_duration: 5s }
      slow_start: 30s
      drain_timeout: 15s
      request_timeout: 30s
      rate_limit: { rps: 50, burst: 100 }
      upstream_tls: { server_name: web.internal }
`
	s, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	lb, err := s.LoadBalancerFor("web")
	if err != nil || lb == nil {
		t.Fatalf("LoadBalancerFor(web) = %v, %v", lb, err)
	}
	if lb.Algorithm != "weighted" || len(lb.Weights) != 3 || lb.ActiveHealth.Path != "/healthz" || lb.RateLimit.Burst != 100 {
		t.Errorf("unexpected config: %+v", lb)
	}
	if _, err := s.LoadBalancerFor("missing"); err == nil {
		t.Error("LoadBalancerFor(missing) must fail")
	}
}

func TestParse_NoLoadBalancerBlock(t *testing.T) {
	s, err := Parse([]byte("version: 1\nservices:\n  web: { build: { type: dockerfile }, port: 80 }\n"))
	if err != nil {
		t.Fatal(err)
	}
	if lb, err := s.LoadBalancerFor("web"); err != nil || lb != nil {
		t.Errorf("LoadBalancerFor = %v, %v, want nil, nil", lb, err)
	}
}

func TestParse_LoadBalancerInvalid(t *testing.T) {
	tests := []struct {
		name string
		lb   string
		want string
	}{
		{"unknown algorithm", "{ algorithm: random }", "algorithm"},
		{"unknown key", "{ sticky: true }", "sticky"},
		{"weights without weighted", "{ weights: [1, 2] }", "weights only apply"},
		{"cookie name without cookie", "{ cookie_name: sid }", "cookie_name"},
		{"health path without slash", "{ active_health: { path: healthz } }", "path"},
		{"bad duration", "{ drain_timeout: soon }", "drain_timeout"},
		{"timeout not below interval", "{ active_health: { path: /h, interval: 2s, timeout: 5s } }", "shorter than interval"},
		{"rate limit without rps", "{ rate_limit: { burst: 5 } }", "rps"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := "version: 1\nservices:\n  web: { build: { type: dockerfile }, port: 80, loadbalancer: " + tt.lb + " }\n"
			_, err := Parse([]byte(doc))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Parse() = %v, want error containing %q", err, tt.want)
			}
		})
	}
}
