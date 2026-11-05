package ingress

import (
	"context"
	"testing"
)

type optOutSrc struct{ off map[string]bool }

func (o optOutSrc) ListAppAutoHostOptOuts(context.Context) (map[string]bool, error) {
	return o.off, nil
}

func TestFallbackResolverSkipsOptedOutApp(t *testing.T) {
	r := LoadFallbackResolver(context.Background(), optOutSrc{off: map[string]bool{"quiet": true}}, "203.0.113.5")
	if _, ok := r.Domain("quiet"); ok {
		t.Fatal("opted-out app got a hostname")
	}
	if h, ok := r.Domain("loud"); !ok || h == "" {
		t.Fatalf("default app lost its hostname: %q %v", h, ok)
	}
	plain := LoadFallbackResolver(context.Background(), struct{}{}, "203.0.113.5")
	if _, ok := plain.Domain("quiet"); !ok {
		t.Fatal("source without opt-outs must keep hostnames")
	}
}
