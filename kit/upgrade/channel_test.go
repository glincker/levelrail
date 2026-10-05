package upgrade

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestValidChannel(t *testing.T) {
	tests := []struct {
		channel string
		want    bool
	}{
		{ChannelStable, true},
		{ChannelBeta, true},
		{ChannelEdge, true},
		{"nightly", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := ValidChannel(tt.channel); got != tt.want {
			t.Errorf("ValidChannel(%q) = %v, want %v", tt.channel, got, tt.want)
		}
	}
}

func fakeFetchers(stable, beta, edge *Release) Fetchers {
	return Fetchers{
		Stable: func(context.Context) (*Release, error) { return stable, nil },
		Beta:   func(context.Context) (*Release, error) { return beta, nil },
		Edge:   func(context.Context) (*Release, error) { return edge, nil },
	}
}

func TestFetchers_LatestForChannel(t *testing.T) {
	stable := &Release{Tag: "v1.0.0"}
	beta := &Release{Tag: "v1.1.0-beta.1"}
	edge := &Release{Tag: "main-abc1234"}
	f := fakeFetchers(stable, beta, edge)

	tests := []struct {
		channel string
		want    *Release
	}{
		{ChannelStable, stable},
		{ChannelBeta, beta},
		{ChannelEdge, edge},
		{"", stable},
		{"unknown", stable},
	}
	for _, tt := range tests {
		got, err := f.LatestForChannel(context.Background(), tt.channel)
		if err != nil {
			t.Fatalf("LatestForChannel(%q) error = %v", tt.channel, err)
		}
		if got != tt.want {
			t.Errorf("LatestForChannel(%q) = %+v, want %+v", tt.channel, got, tt.want)
		}
	}
}

func TestFetchers_LatestForChannel_PropagatesError(t *testing.T) {
	wantErr := errors.New("network unreachable")
	f := Fetchers{
		Stable: func(context.Context) (*Release, error) { return nil, wantErr },
		Beta:   func(context.Context) (*Release, error) { return nil, nil },
		Edge:   func(context.Context) (*Release, error) { return nil, nil },
	}
	_, err := f.LatestForChannel(context.Background(), ChannelStable)
	if !errors.Is(err, wantErr) {
		t.Errorf("LatestForChannel() error = %v, want %v", err, wantErr)
	}
}

func TestUpdateAvailable(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  *Release
		want    bool
	}{
		{"dev build never reports available", "dev", &Release{Tag: "v1.1.0"}, false},
		{"no release known", "v1.0.0", nil, false},
		{"already on latest", "v1.1.0", &Release{Tag: "v1.1.0"}, false},
		{"newer available", "v1.0.0", &Release{Tag: "v1.1.0"}, true},
		{"beta.9 older than beta.15", "v0.2.0-beta.15", &Release{Tag: "v0.2.0-beta.9"}, false},
		{"beta.14 to beta.15", "v0.2.0-beta.14", &Release{Tag: "v0.2.0-beta.15"}, true},
		{"beta.15 up to date", "v0.2.0-beta.15", &Release{Tag: "v0.2.0-beta.15"}, false},
		{"beta older than its stable", "v0.2.0-beta.15", &Release{Tag: "v0.2.0"}, true},
		{"stable not downgraded to beta", "v0.2.0", &Release{Tag: "v0.2.0-beta.15"}, false},
		{"edge sha falls back to inequality", "main-aaaaaaa", &Release{Tag: "main-bbbbbbb"}, true},
		{"edge same sha is up to date", "main-aaaaaaa", &Release{Tag: "main-aaaaaaa"}, false},
		{"release build vs edge sha falls back", "v1.0.0", &Release{Tag: "main-bbbbbbb"}, true},
		{"dev never updates to edge sha", "dev", &Release{Tag: "main-bbbbbbb"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UpdateAvailable(tt.current, tt.latest); got != tt.want {
				t.Errorf("UpdateAvailable(%q, %+v) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

// TestPickLatestPrerelease_OutOfOrderList is the regression test for a
// live bug: GitHub's /releases list is ordered by internal release id,
// not publish time, so the raw order on a real repo
// put v0.2.0-beta.9 ahead of the actually-newer v0.2.0-beta.14. Taking
// the first prerelease match reported a nine-release-old "latest".
func TestPickLatestPrerelease_OutOfOrderList(t *testing.T) {
	rrs := []rawRelease{
		{TagName: "v0.2.0-beta.9", Prerelease: true, PublishedAt: "2026-09-23T04:27:56Z"},
		{TagName: "v0.2.0-beta.8", Prerelease: true, PublishedAt: "2026-09-23T04:15:35Z"},
		{TagName: "v0.2.0-beta.14", Prerelease: true, PublishedAt: "2026-09-23T22:12:59Z"},
		{TagName: "v0.2.0-beta.13", Prerelease: true, PublishedAt: "2026-09-23T20:50:05Z"},
		{TagName: "v0.2.0-beta.5", Prerelease: true, PublishedAt: "2026-09-21T19:16:56Z"},
	}

	got := pickLatestPrerelease(rrs)
	if got == nil || got.Tag != "v0.2.0-beta.14" {
		t.Fatalf("pickLatestPrerelease() = %+v, want tag v0.2.0-beta.14", got)
	}
}

func TestPickLatestPrerelease_SkipsDraftsAndStable(t *testing.T) {
	rrs := []rawRelease{
		{TagName: "v0.3.0", Prerelease: false, PublishedAt: "2026-09-24T00:00:00Z"},
		{TagName: "v0.3.0-beta.2", Prerelease: true, Draft: true, PublishedAt: "2026-09-25T00:00:00Z"},
		{TagName: "v0.3.0-beta.1", Prerelease: true, PublishedAt: "2026-09-23T00:00:00Z"},
	}

	got := pickLatestPrerelease(rrs)
	if got == nil || got.Tag != "v0.3.0-beta.1" {
		t.Fatalf("pickLatestPrerelease() = %+v, want tag v0.3.0-beta.1", got)
	}
}

func TestPickLatestPrerelease_NoneMatch(t *testing.T) {
	rrs := []rawRelease{{TagName: "v0.3.0", Prerelease: false, PublishedAt: "2026-09-24T00:00:00Z"}}
	if got := pickLatestPrerelease(rrs); got != nil {
		t.Fatalf("pickLatestPrerelease() = %+v, want nil", got)
	}
}

func TestCache_FreshStaleSet(t *testing.T) {
	c := NewCache()

	if _, ok := c.Fresh(time.Hour, ChannelStable); ok {
		t.Fatal("Fresh() on an empty cache = true, want false")
	}
	if _, ok := c.Stale(ChannelStable); ok {
		t.Fatal("Stale() on an empty cache = true, want false")
	}

	release := &Release{Tag: "v1.2.3"}
	c.Set(ChannelStable, release)

	got, ok := c.Fresh(time.Hour, ChannelStable)
	if !ok || got != release {
		t.Fatalf("Fresh(stable) = %+v, %v, want %+v, true", got, ok, release)
	}

	if _, ok := c.Fresh(time.Hour, ChannelBeta); ok {
		t.Error("Fresh(beta) = true after only stable was cached, want false: channel mismatch must not serve stale data")
	}
	if _, ok := c.Stale(ChannelBeta); ok {
		t.Error("Stale(beta) = true after only stable was cached, want false")
	}

	if got, ok := c.Fresh(0, ChannelStable); ok {
		t.Errorf("Fresh() with a zero TTL = %+v, true, want false (immediately expired)", got)
	}
	got, ok = c.Stale(ChannelStable)
	if !ok || got != release {
		t.Errorf("Stale(stable) after TTL expiry = %+v, %v, want %+v, true", got, ok, release)
	}
}
