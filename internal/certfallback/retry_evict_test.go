package certfallback

import (
	"context"
	"testing"
)

func TestRetrierEvictsCacheBeforeNudge(t *testing.T) {
	s := &memStore{}
	s.seed(t, "local", "up.example.dev")
	var order []string
	r := &Retrier{
		Store:      s,
		Candidates: func(context.Context) []string { return []string{"up.example.dev"} },
		Resolves:   func(context.Context, string) bool { return true },
		MaxRetries: func(context.Context) int { return 2 },
		Evict:      func(context.Context) error { order = append(order, "evict"); return nil },
		Nudge:      func() { order = append(order, "nudge") },
	}
	if got, err := r.Pass(context.Background()); err != nil || len(got) != 1 {
		t.Fatalf("Pass = %v, %v", got, err)
	}
	if len(order) != 2 || order[0] != "evict" || order[1] != "nudge" {
		t.Errorf("order = %v, want evict then nudge", order)
	}
}
