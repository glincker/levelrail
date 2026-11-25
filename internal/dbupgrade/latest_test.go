package dbupgrade

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeTags struct {
	tags  []string
	err   error
	calls int
}

func (f *fakeTags) ListTags(context.Context, string) ([]string, error) {
	f.calls++
	return f.tags, f.err
}

func TestLatestResolver(t *testing.T) {
	now := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name, engine string
		tags         []string
		err          error
		want, src    string
		skipped      string
	}{
		{"postgres filters variants and prereleases", "postgres", []string{"17.7", "18.1", "18.1-alpine", "18.2-rc1", "18beta2", "latest", "16.11"}, nil, "18.1", "registry", ""},
		{"postgres unknown newer major is not picked", "postgres", []string{"18.1", "18.3", "99.0"}, nil, "18.3", "registry", "99.0"},
		{"postgres EOL line skipped", "postgres", []string{"13.23"}, nil, "16", "fallback", ""},
		{"most specific tag wins on equal version", "postgres", []string{"18", "18.1", "18.1.0"}, nil, "18.1.0", "registry", ""},
		{"mysql needs a listed line", "mysql", []string{"8.0.43", "8.4.6", "9.1.0", "windowsservercore-ltsc2022"}, nil, "8.4.6", "registry", "9.1.0"},
		{"outage falls back", "postgres", nil, errors.New("down"), "16", "fallback", ""},
		{"clickhouse with every old line EOL uses the newest listed line", "clickhouse", []string{"25.8.3", "26.8.44", "26.3.69", "26.10.11"}, nil, "26.8.44", "registry", ""},
		{"no stable tag falls back", "redis", []string{"latest", "9.0-rc1"}, nil, "7", "fallback", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &LatestResolver{Now: func() time.Time { return now }}
			fb := map[string]string{"postgres": "16", "mysql": "8", "redis": "7", "clickhouse": "24.8"}[c.engine]
			got := r.Resolve(context.Background(), &fakeTags{tags: c.tags, err: c.err}, c.engine, c.engine, fb, time.Hour)
			if got.Version != c.want || got.Source != c.src || got.SkippedNewer != c.skipped {
				t.Fatalf("got %+v, want version %q source %q skipped %q", got, c.want, c.src, c.skipped)
			}
		})
	}
}

func TestLatestResolverCacheTTL(t *testing.T) {
	now := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	f := &fakeTags{tags: []string{"18.1"}}
	r := &LatestResolver{Now: func() time.Time { return now }}
	r.Resolve(context.Background(), f, "postgres", "postgres", "16", time.Hour)
	if got := r.Resolve(context.Background(), f, "postgres", "postgres", "16", time.Hour); got.Source != LatestSourceCache || f.calls != 1 {
		t.Fatalf("second call = %+v calls=%d", got, f.calls)
	}
	now = now.Add(2 * time.Hour)
	r.Resolve(context.Background(), f, "postgres", "postgres", "16", time.Hour)
	if f.calls != 2 {
		t.Fatalf("calls after ttl = %d, want 2", f.calls)
	}
}

func TestLatestResolverServesStaleOnErrorAndBacksOff(t *testing.T) {
	now := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	f := &fakeTags{tags: []string{"18.1"}}
	r := &LatestResolver{Now: func() time.Time { return now }}
	r.Resolve(context.Background(), f, "postgres", "postgres", "16", time.Minute)
	now = now.Add(2 * time.Minute)
	f.err = errors.New("down")
	got := r.Resolve(context.Background(), f, "postgres", "postgres", "16", time.Minute)
	if got.Version != "18.1" {
		t.Fatalf("stale result expected, got %+v", got)
	}
	calls := f.calls
	r.Resolve(context.Background(), f, "postgres", "postgres", "16", time.Minute)
	if f.calls != calls {
		t.Fatalf("failure was not remembered: calls %d -> %d", calls, f.calls)
	}
	now = now.Add(latestFailureBackoff + time.Second)
	r.Resolve(context.Background(), f, "postgres", "postgres", "16", time.Minute)
	if f.calls != calls+1 {
		t.Fatalf("lookup not retried after backoff: %d", f.calls)
	}
}
