package ingress

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/loadbalancer"
)

func TestNewLBRoute(t *testing.T) {
	ups := []loadbalancer.Upstream{{Dial: "10.0.0.1:80"}, {Dial: "10.0.0.2:80"}}
	if NewLBRoute(loadbalancer.Config{}, nil, nil) != nil {
		t.Fatal("no upstreams must give nil route")
	}
	lb := NewLBRoute(loadbalancer.Config{
		Algorithm:      loadbalancer.AlgoCookie,
		ActiveHealth:   &loadbalancer.ActiveHealth{Path: "/h"},
		PassiveHealth:  &loadbalancer.PassiveHealth{},
		Retries:        &loadbalancer.Retries{Count: 2},
		DrainTimeout:   "10s",
		RequestTimeout: "20s",
		RateLimit:      &loadbalancer.RateLimit{RPS: 4},
		UpstreamTLS:    &loadbalancer.UpstreamTLS{InsecureSkipVerify: true},
	}, ups, nil)
	if lb.Policy != LBPolicyCookie || lb.CookieName != "lb" || lb.ActiveHealth.Interval != "10s" || lb.PassiveHealth.MaxFails != 1 ||
		lb.Retries != 2 || lb.TryDuration != "5s" || lb.StreamCloseDelay != "10s" || lb.ResponseHeaderTimeout != "20s" ||
		lb.RateLimitRPS != 4 || !lb.UpstreamTLS.InsecureSkipVerify || len(lb.Upstreams) != 2 {
		t.Fatalf("unexpected route: %+v", lb)
	}
	w := NewLBRoute(loadbalancer.Config{Algorithm: loadbalancer.AlgoWeighted}, ups, []int{2, 1})
	if w.Policy != LBPolicyWeighted || len(w.Weights) != 2 {
		t.Fatalf("weighted route: %+v", w)
	}
}
