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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UpdateAvailable(tt.current, tt.latest); got != tt.want {
				t.Errorf("UpdateAvailable(%q, %+v) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
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
