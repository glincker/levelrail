package api

import (
	"context"
	"errors"
	"testing"

	"github.com/glincker/levelrail/internal/selfupgrade"
	"github.com/glincker/levelrail/kit/upgrade"
)

func TestDefaultFetchLatestRelease(t *testing.T) {
	orig, origDiscover := fetchStableRelease, discoverTag
	t.Cleanup(func() { fetchStableRelease, discoverTag = orig, origDiscover })
	discoverTag = func(context.Context) (selfupgrade.Discovered, error) {
		return selfupgrade.Discovered{}, errors.New("no web source")
	}

	tests := []struct {
		name    string
		rel     *upgrade.Release
		err     error
		wantTag string
		wantNil bool
		wantErr bool
		assets  int
	}{
		{name: "product release with assets", rel: &upgrade.Release{Tag: "v0.2.0", URL: "https://example.test/r", AssetNames: []string{"levelrail-linux-amd64", "checksums.txt"}}, wantTag: "v0.2.0", assets: 2},
		{name: "no release published", rel: nil, wantNil: true},
		{name: "fetch failure is wrapped", err: errors.New("boom"), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fetchStableRelease = func(context.Context, string) (*upgrade.Release, error) { return tc.rel, tc.err }
			got, err := defaultFetchLatestRelease(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantNil || tc.wantErr {
				if got != nil {
					t.Fatalf("got %+v, want nil", got)
				}
				return
			}
			if got == nil || got.TagName != tc.wantTag || len(got.Assets) != tc.assets {
				t.Fatalf("got %+v, want tag %s with %d assets", got, tc.wantTag, tc.assets)
			}
		})
	}
}

func TestLatestReleaseFallsBackToTheReleasePages(t *testing.T) {
	orig, origDiscover := fetchStableRelease, discoverTag
	t.Cleanup(func() { fetchStableRelease, discoverTag = orig, origDiscover })
	fetchStableRelease = func(context.Context, string) (*upgrade.Release, error) { return nil, errors.New("api blocked") }
	discoverTag = func(context.Context) (selfupgrade.Discovered, error) {
		return selfupgrade.Discovered{Tag: "v1.5.8", URL: "https://github.com/glincker/levelrail/releases/tag/v1.5.8", PublishedAt: "2026-11-18T10:00:00Z", Source: "atom"}, nil
	}
	got, err := defaultFetchLatestRelease(context.Background())
	if err != nil {
		t.Fatalf("fallback should answer: %v", err)
	}
	if got.TagName != "v1.5.8" || !got.Fallback || got.Body != "" || len(got.Assets) != 0 {
		t.Fatalf("fallback release = %+v, want tag only, flagged, no notes", got)
	}

	cfg := &selfUpgradeConfig{
		notes:    func(context.Context) ([]upgrade.HistoryRelease, error) { return nil, errors.New("api blocked") },
		discover: discoverStableRelease,
	}
	list, fromFallback, err := cfg.releasesWithSource(context.Background())
	if err != nil || !fromFallback || len(list) != 1 || list[0].Tag != "v1.5.8" || list[0].Body != "" {
		t.Fatalf("release list = %+v fallback=%v err=%v", list, fromFallback, err)
	}
	if cfg.cached != nil {
		t.Fatal("a fallback answer must not be cached as a good list")
	}
}

// No test may reach the public release pages; tests that need a fallback set their own.
func init() {
	discoverTag = func(context.Context) (selfupgrade.Discovered, error) {
		return selfupgrade.Discovered{}, errors.New("release pages are disabled in tests")
	}
}
