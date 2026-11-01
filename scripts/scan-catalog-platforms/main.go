// Command scan-catalog-platforms inspects every catalog image in the registry
// and records which platforms its manifest lists in internal/catalog/platforms.json.
// Run it by hand or on a schedule; it never runs at build or deploy time.
//
// Env: PLATFORM_SCAN_CONCURRENCY, PLATFORM_SCAN_TIMEOUT_SEC, PLATFORM_SCAN_RATE_PER_SEC,
// PLATFORM_SCAN_TTL_HOURS (skip fresh entries), PLATFORM_SCAN_MAX (0 = all), PLATFORM_SCAN_OUT.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

// errRateLimited stops the whole run so partial results are saved instead of hammering a registry.
var errRateLimited = errors.New("registry rate limit reached")

// Fetcher returns the os/arch platforms of one image reference.
type Fetcher func(ctx context.Context, ref string) ([]string, error)

type config struct {
	Concurrency int
	Timeout     time.Duration
	RatePerSec  float64
	TTL         time.Duration
	Max         int
	Out         string
	Only        map[string]bool
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v >= 0 {
		return v
	}
	return def
}

func loadConfig() config {
	rate := 2.0
	if v, err := strconv.ParseFloat(os.Getenv("PLATFORM_SCAN_RATE_PER_SEC"), 64); err == nil && v > 0 {
		rate = v
	}
	out := os.Getenv("PLATFORM_SCAN_OUT")
	if out == "" {
		out = "internal/catalog/platforms.json"
	}
	only := map[string]bool{}
	for _, r := range strings.Split(os.Getenv("PLATFORM_SCAN_ONLY"), ",") {
		if r = strings.TrimSpace(r); r != "" {
			only[r] = true
		}
	}
	return config{
		Only:        only,
		Concurrency: max(1, envInt("PLATFORM_SCAN_CONCURRENCY", 4)),
		Timeout:     time.Duration(max(1, envInt("PLATFORM_SCAN_TIMEOUT_SEC", 45))) * time.Second,
		RatePerSec:  rate,
		TTL:         time.Duration(envInt("PLATFORM_SCAN_TTL_HOURS", 168)) * time.Hour,
		Max:         envInt("PLATFORM_SCAN_MAX", 0),
		Out:         out,
	}
}

// platformsFromRaw parses an index manifest. ok is false for a single manifest.
func platformsFromRaw(raw []byte) ([]string, bool) {
	var idx struct {
		Manifests []struct {
			Platform *struct {
				OS           string `json:"os"`
				Architecture string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(raw, &idx); err != nil || len(idx.Manifests) == 0 {
		return nil, false
	}
	set := map[string]bool{}
	for _, m := range idx.Manifests {
		if m.Platform == nil || m.Platform.OS == "unknown" || m.Platform.Architecture == "unknown" {
			continue
		}
		set[m.Platform.OS+"/"+m.Platform.Architecture] = true
	}
	return sortedKeys(set), true
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// wantedRefs is every distinct literal image in the catalog, sorted.
func wantedRefs(templates []catalog.Template) []string {
	set := map[string]bool{}
	for _, t := range templates {
		imgs, _ := catalog.ComposeImages(t.Compose)
		for _, i := range imgs {
			set[i] = true
		}
	}
	return sortedKeys(set)
}

// scan checks refs through fetch, reusing fresh entries from prev. It returns
// the merged entries and whether the run stopped early on a rate limit.
func scan(ctx context.Context, cfg config, prev []catalog.ImagePlatforms, refs []string, fetch Fetcher, now time.Time) ([]catalog.ImagePlatforms, bool) {
	known := map[string]catalog.ImagePlatforms{}
	for _, p := range prev {
		known[p.Ref()] = p
	}
	var todo []string
	for _, ref := range refs {
		if len(cfg.Only) > 0 {
			if cfg.Only[ref] {
				todo = append(todo, ref)
			}
			continue
		}
		if p, ok := known[ref]; ok && p.Error == "" && fresh(p, cfg.TTL, now) {
			continue
		}
		todo = append(todo, ref)
	}
	if cfg.Max > 0 && len(todo) > cfg.Max {
		todo = todo[:cfg.Max]
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	limited := false
	for w := 0; w < cfg.Concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ref := range jobs {
				cctx, ccancel := context.WithTimeout(ctx, cfg.Timeout)
				plats, err := fetch(cctx, ref)
				ccancel()
				img, tag := catalog.SplitImageRef(ref)
				entry := catalog.ImagePlatforms{Image: img, Tag: tag, Platforms: plats, CheckedAt: now.UTC().Format(time.RFC3339)}
				mu.Lock()
				switch {
				case errors.Is(err, errRateLimited):
					limited = true
					cancel()
				case ctx.Err() != nil:
					// run cancelled mid-flight: keep any older entry
				case err != nil:
					entry.Platforms = nil
					entry.Error = err.Error()
					known[ref] = entry
				default:
					known[ref] = entry
				}
				mu.Unlock()
			}
		}()
	}
	interval := time.Duration(float64(time.Second) / cfg.RatePerSec)
feed:
	for i, ref := range todo {
		if i > 0 && interval > 0 {
			select {
			case <-time.After(interval):
			case <-ctx.Done():
				break feed
			}
		}
		select {
		case jobs <- ref:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()

	// Keep only refs the catalog still uses, so removed templates age out.
	wantSet := map[string]bool{}
	for _, r := range refs {
		wantSet[r] = true
	}
	out := make([]catalog.ImagePlatforms, 0, len(known))
	for ref, p := range known {
		if wantSet[ref] {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref() < out[j].Ref() })
	return out, limited
}

func fresh(p catalog.ImagePlatforms, ttl time.Duration, now time.Time) bool {
	t, err := time.Parse(time.RFC3339, p.CheckedAt)
	return err == nil && now.Sub(t) < ttl
}

// templatePlatforms is the intersection of each template's image platforms;
// any unverified image leaves the template out (unknown).
func templatePlatforms(templates []catalog.Template, images []catalog.ImagePlatforms) map[string][]string {
	byRef := map[string]catalog.ImagePlatforms{}
	for _, p := range images {
		byRef[p.Ref()] = p
	}
	out := map[string][]string{}
	for _, t := range templates {
		imgs, ok := catalog.ComposeImages(t.Compose)
		if !ok || len(imgs) == 0 {
			continue
		}
		var inter map[string]bool
		verified := true
		for _, ref := range imgs {
			p, found := byRef[ref]
			if !found || p.Error != "" || len(p.Platforms) == 0 {
				verified = false
				break
			}
			cur := map[string]bool{}
			for _, pl := range p.Platforms {
				if inter == nil || inter[pl] {
					cur[pl] = true
				}
			}
			inter = cur
		}
		if verified && len(inter) > 0 {
			out[t.ID] = sortedKeys(inter)
		}
	}
	return out
}

func main() {
	cfg := loadConfig()
	var prev catalog.PlatformsFile
	if b, err := os.ReadFile(cfg.Out); err == nil {
		_ = json.Unmarshal(b, &prev)
	}
	refs := wantedRefs(catalog.Templates)
	now := time.Now()
	images, limited := scan(context.Background(), cfg, prev.Images, refs, registryFetcher(&http.Client{Timeout: cfg.Timeout}), now)
	file := catalog.PlatformsFile{
		ScannedAt: now.UTC().Format(time.RFC3339),
		Images:    images,
		Templates: templatePlatforms(catalog.Templates, images),
	}
	b, _ := json.MarshalIndent(file, "", "  ")
	if err := os.WriteFile(cfg.Out, append(b, '\n'), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	var arm, amdOnly, unknown int
	for _, p := range images {
		switch {
		case p.Error != "" || len(p.Platforms) == 0:
			unknown++
		case contains(p.Platforms, "linux/arm64"):
			arm++
		default:
			amdOnly++
		}
	}
	fmt.Printf("images wanted %d, checked %d: arm64 %d, no arm64 %d, failed %d, templates with data %d\n",
		len(refs), len(images), arm, amdOnly, unknown, len(file.Templates))
	if limited {
		fmt.Println("stopped early on a registry rate limit; partial results saved, rerun later")
		os.Exit(3)
	}
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
