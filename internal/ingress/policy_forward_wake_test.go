package ingress

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func TestForwardHandleAppStates(t *testing.T) {
	rule := trafficpolicy.Forwarder{
		Match:       trafficpolicy.Match{Kind: trafficpolicy.MatchPrefix, Path: "/fn"},
		Action:      trafficpolicy.ActionApp,
		App:         "hello",
		StripPrefix: true,
	}
	wake := WakeRoute{Dial: "127.0.0.1:1", Token: "t", App: "hello"}
	tests := []struct {
		name       string
		policy     RoutePolicy
		wantLen    int
		wantWakeRW bool
	}{
		{"running proxies with strip", RoutePolicy{AppDials: map[string]string{"hello": "127.0.0.1:9"}}, 2, false},
		{"asleep wakes the app", RoutePolicy{AppWake: map[string]WakeRoute{"hello": wake}}, 2, true},
		{"running wins over a stale wake entry", RoutePolicy{AppDials: map[string]string{"hello": "127.0.0.1:9"}, AppWake: map[string]WakeRoute{"hello": wake}}, 2, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.policy.forwardHandle(rule)
			if len(got) != tc.wantLen {
				t.Fatalf("handlers = %d, want %d", len(got), tc.wantLen)
			}
			_, isWake := got[0].(RewriteHandler)
			if isWake != tc.wantWakeRW {
				t.Fatalf("first handler %T, wake rewrite = %v, want %v", got[0], isWake, tc.wantWakeRW)
			}
		})
	}
}
