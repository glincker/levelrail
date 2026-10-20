package build

import (
	"context"
	"sync"
	"testing"
)

func TestLimiterConcurrencyIsPerNode(t *testing.T) {
	tests := []struct {
		name        string
		limit       int
		nodes       []string
		wantWaiting int
		wantRunning int
	}{
		{name: "same node queues past the limit", limit: 1, nodes: []string{"a", "a"}, wantWaiting: 1, wantRunning: 1},
		{name: "different nodes run side by side", limit: 1, nodes: []string{"a", "b"}, wantWaiting: 0, wantRunning: 2},
		{name: "limit 2 admits two on one node", limit: 2, nodes: []string{"a", "a", "a"}, wantWaiting: 1, wantRunning: 2},
		{name: "zero means unlimited", limit: 0, nodes: []string{"a", "a", "a"}, wantWaiting: 0, wantRunning: 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			release := make(chan struct{})
			entered := make(chan struct{}, len(tc.nodes))
			b := fakeBuilder{fn: func(ctx context.Context, _ func(ProgressEvent)) (*Result, error) {
				entered <- struct{}{}
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				return &Result{}, nil
			}}
			next := make(chan string, len(tc.nodes))
			for _, n := range tc.nodes {
				next <- n
			}
			l := NewLimiter(b, fixed(Limits{MaxConcurrent: tc.limit}),
				WithNodeResolver(func(context.Context) string { return <-next }),
				WithImageSizer(func(context.Context, string) int64 { return 7 }))

			var wg sync.WaitGroup
			for range tc.nodes {
				wg.Add(1)
				go func() { defer wg.Done(); _, _ = l.Build(context.Background(), Request{Tag: "web:a"}, nil) }()
			}
			for i := 0; i < tc.wantRunning; i++ {
				<-entered
			}
			waitFor(t, func() bool { w, r := l.Queued(); return w == tc.wantWaiting && r == tc.wantRunning })
			close(release)
			wg.Wait()
			if w, r := l.Queued(); w != 0 || r != 0 {
				t.Fatalf("leaked slots: waiting=%d running=%d", w, r)
			}
		})
	}
}

func TestLimiterRecordsNodePlatformAndImageSize(t *testing.T) {
	rec := &memRecorder{}
	b := fakeBuilder{fn: func(context.Context, func(ProgressEvent)) (*Result, error) { return &Result{}, nil }}
	l := NewLimiter(b, fixed(Limits{MaxConcurrent: 1}), WithRunRecorder(rec),
		WithNodeResolver(func(context.Context) string { return "node-1" }),
		WithImageSizer(func(_ context.Context, tag string) int64 {
			if tag != "web:a" {
				t.Errorf("sized tag %q", tag)
			}
			return 4096
		}))
	if _, err := l.Build(context.Background(), Request{Tag: "web:a"}, nil); err != nil {
		t.Fatal(err)
	}
	got := rec.runs[0]
	if got.Node != "node-1" || got.ImageBytes != 4096 || got.Platform != HostPlatform() {
		t.Fatalf("run = %+v", got)
	}
	if load := l.Load(); len(load) != 1 || load[0].Node != "node-1" || load[0].Limit != 1 {
		t.Fatalf("load = %+v", load)
	}
}
