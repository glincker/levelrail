package monitor

import (
	"context"
	"testing"
	"time"
)

func TestHostOnlyKindsValidateAndResolve(t *testing.T) {
	cfg := Config{MinIntervalTCP: 30 * time.Second, MaxTimeout: 30 * time.Second}
	for _, kind := range []string{KindDNS, KindPing} {
		m := Normalize(Monitor{Name: "n", Kind: kind, Interval: time.Minute, Timeout: 5 * time.Second, Spec: Spec{Host: "example.com"}})
		if err := Validate(m, cfg); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		m.Spec.Host = "example.com:53"
		if err := Validate(m, cfg); err == nil {
			t.Fatalf("%s: host with a port must be refused", kind)
		}
	}
	c := NewChecker(Config{})
	var res Result
	c.checkDNS(context.Background(), Monitor{Spec: Spec{Host: "localhost"}}, &res)
	if !res.OK {
		t.Fatalf("localhost should resolve, reason %q", res.Reason)
	}
}
