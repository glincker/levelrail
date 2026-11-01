package main

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

func TestPlatformsFromRaw(t *testing.T) {
	raw := []byte(`{"manifests":[{"platform":{"os":"linux","architecture":"amd64"}},{"platform":{"os":"linux","architecture":"arm64","variant":"v8"}},{"platform":{"os":"unknown","architecture":"unknown"}}]}`)
	got, ok := platformsFromRaw(raw)
	if !ok || len(got) != 2 || got[0] != "linux/amd64" || got[1] != "linux/arm64" {
		t.Fatalf("got %v ok=%v", got, ok)
	}
	if _, ok := platformsFromRaw([]byte(`{"config":{},"layers":[]}`)); ok {
		t.Fatal("single manifest must not parse as index")
	}
}

func fakeCfg() config {
	return config{Concurrency: 2, Timeout: time.Second, RatePerSec: 1000, TTL: time.Hour}
}

func TestScanReusesFreshAndRecordsErrors(t *testing.T) {
	now := time.Now()
	prev := []catalog.ImagePlatforms{{Image: "a", Tag: "1", Platforms: []string{"linux/amd64"}, CheckedAt: now.Format(time.RFC3339)}}
	calls := 0
	fetch := func(_ context.Context, ref string) ([]string, error) {
		calls++
		if ref == "bad:1" {
			return nil, context.DeadlineExceeded
		}
		return []string{"linux/amd64", "linux/arm64"}, nil
	}
	out, limited := scan(context.Background(), fakeCfg(), prev, []string{"a:1", "b:1", "bad:1"}, fetch, now)
	if limited || calls != 2 || len(out) != 3 {
		t.Fatalf("limited=%v calls=%d out=%v", limited, calls, out)
	}
	if out[2].Error == "" || len(out[2].Platforms) != 0 {
		t.Fatalf("failure not recorded as unknown: %+v", out[2])
	}
}

func TestScanStopsOnRateLimit(t *testing.T) {
	cfg := fakeCfg()
	cfg.Concurrency = 1
	fetch := func(_ context.Context, ref string) ([]string, error) {
		if ref == "b:1" {
			return nil, errRateLimited
		}
		return []string{"linux/arm64"}, nil
	}
	out, limited := scan(context.Background(), cfg, nil, []string{"a:1", "b:1", "c:1", "d:1"}, fetch, time.Now())
	if !limited {
		t.Fatal("expected rate limit stop")
	}
	if len(out) != 1 || out[0].Ref() != "a:1" {
		t.Fatalf("partial results wrong: %v", out)
	}
}

func TestTemplatePlatformsIntersectsAndSkipsUnknown(t *testing.T) {
	tpls := []catalog.Template{
		{ID: "both", Compose: "services:\n  a:\n    image: a:1\n  b:\n    image: b:1\n"},
		{ID: "unk", Compose: "services:\n  a:\n    image: a:1\n  b:\n    image: nope:1\n"},
		{ID: "var", Compose: "services:\n  a:\n    image: a:${V}\n"},
	}
	imgs := []catalog.ImagePlatforms{
		{Image: "a", Tag: "1", Platforms: []string{"linux/amd64", "linux/arm64"}},
		{Image: "b", Tag: "1", Platforms: []string{"linux/amd64"}},
	}
	got := templatePlatforms(tpls, imgs)
	if len(got) != 1 || len(got["both"]) != 1 || got["both"][0] != "linux/amd64" {
		t.Fatalf("got %v", got)
	}
}

func TestScanOnlyFetchesRequestedAndKeepsRest(t *testing.T) {
	now := time.Now()
	old := now.Add(-1000 * time.Hour).Format(time.RFC3339)
	prev := []catalog.ImagePlatforms{{Image: "a", Tag: "1", Platforms: []string{"linux/amd64"}, CheckedAt: old}}
	var fetched []string
	fetch := func(_ context.Context, ref string) ([]string, error) {
		fetched = append(fetched, ref)
		return []string{"linux/amd64", "linux/arm64"}, nil
	}
	cfg := fakeCfg()
	cfg.Only = map[string]bool{"b:1": true}
	out, _ := scan(context.Background(), cfg, prev, []string{"a:1", "b:1", "c:1"}, fetch, now)
	if len(fetched) != 1 || fetched[0] != "b:1" {
		t.Fatalf("fetched %v, want only b:1", fetched)
	}
	if len(out) != 2 || out[0].Ref() != "a:1" || out[1].Ref() != "b:1" {
		t.Fatalf("out %v", out)
	}
}

func TestAddTemplateRefsScopesByTemplateID(t *testing.T) {
	templates := []catalog.Template{
		{ID: "x", Compose: "services:\n  a:\n    image: img/x:1\n"},
		{ID: "y", Compose: "services:\n  a:\n    image: img/y:1\n"},
	}
	only := map[string]bool{}
	addTemplateRefs(only, " x ,", templates)
	if len(only) != 1 || !only["img/x:1"] {
		t.Fatalf("only %v", only)
	}
}
