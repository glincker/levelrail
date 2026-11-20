package api

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

func TestChannelReleaseMirrorFallback(t *testing.T) {
	orig := discoverMirror
	t.Cleanup(func() { discoverMirror = orig })
	blocked := errors.New("blocked")
	tests := []struct {
		name       string
		channel    string
		base       string
		mirrorTag  string
		mirrorErr  error
		wantTag    string
		wantOK     bool
		wantStale  bool
		wantMirror bool
		wantEdge   bool
	}{
		{name: "beta from mirror", channel: upgrade.ChannelBeta, base: "https://m.test", mirrorTag: "v1.6.0-beta.2", wantTag: "v1.6.0-beta.2", wantOK: true, wantMirror: true},
		{name: "edge from mirror", channel: upgrade.ChannelEdge, base: "https://m.test", mirrorTag: "main-abc1234", wantTag: "main-abc1234", wantOK: true, wantMirror: true, wantEdge: true},
		{name: "no mirror is stale", channel: upgrade.ChannelBeta, wantStale: true},
		{name: "mirror failure is stale", channel: upgrade.ChannelBeta, base: "https://m.test", mirrorErr: blocked, wantStale: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			discoverMirror = func(_ context.Context, _, _ string) (selfupgrade.Discovered, error) {
				return selfupgrade.Discovered{Tag: tc.mirrorTag}, tc.mirrorErr
			}
			rt := &Router{channelUpdatesCache: upgrade.NewCache()}
			rt.upgradeFetchers = upgrade.Fetchers{
				Beta: func(context.Context) (*upgrade.Release, error) { return nil, blocked },
				Edge: func(context.Context) (*upgrade.Release, error) { return nil, blocked },
			}
			got, ok := rt.channelReleaseVia(context.Background(), tc.channel, func() string { return tc.base })
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && (got.Tag != tc.wantTag || got.IsEdge() != tc.wantEdge) {
				t.Fatalf("got %+v", got)
			}
			if rt.channels.isStale(tc.channel) != tc.wantStale {
				t.Fatalf("stale = %v, want %v", rt.channels.isStale(tc.channel), tc.wantStale)
			}
			if rt.channels.fromMirror(tc.channel, tc.wantTag) != tc.wantMirror {
				t.Fatalf("fromMirror = %v, want %v", !tc.wantMirror, tc.wantMirror)
			}
		})
	}
}

func TestStableReleaseUsesMirrorWhenPagesBlocked(t *testing.T) {
	origF, origD, origM := fetchStableRelease, discoverTag, discoverMirror
	t.Cleanup(func() { fetchStableRelease, discoverTag, discoverMirror = origF, origD, origM })
	fetchStableRelease = func(context.Context, string) (*upgrade.Release, error) { return nil, errors.New("api blocked") }
	discoverTag = func(context.Context) (selfupgrade.Discovered, error) {
		return selfupgrade.Discovered{}, errors.New("pages blocked")
	}
	discoverMirror = func(context.Context, string, string) (selfupgrade.Discovered, error) {
		return selfupgrade.Discovered{Tag: "v1.5.9", Source: "mirror"}, nil
	}
	if _, err := fetchLatestReleaseVia(context.Background(), ""); err == nil {
		t.Fatal("no mirror set should fail")
	}
	got, err := fetchLatestReleaseVia(context.Background(), "https://m.test")
	if err != nil || got.TagName != "v1.5.9" || !got.Fallback {
		t.Fatalf("got %+v, err %v", got, err)
	}
}
