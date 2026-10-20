package dbupgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
)

// EnvRefreshInterval sets how often registry tags are refreshed; "0" disables it.
const EnvRefreshInterval = "APP_DB_VERSION_REFRESH_INTERVAL"

// DefaultRefreshInterval is daily: database releases ship monthly at most.
const DefaultRefreshInterval = 24 * time.Hour

const (
	dockerHubTagsURL = "https://hub.docker.com/v2/repositories/%s/tags?page_size=100&ordering=last_updated"
	maxTagPages      = 3
	tagFetchTimeout  = 20 * time.Second
)

// TagLister lists the tags of one image repository.
type TagLister interface {
	ListTags(ctx context.Context, repository string) ([]string, error)
}

// DockerHubLister reads tags from Docker Hub's public tag API.
type DockerHubLister struct {
	Client *http.Client
}

type hubTagsPage struct {
	Next    string `json:"next"`
	Results []struct {
		Name string `json:"name"`
	} `json:"results"`
}

// ListTags returns up to maxTagPages pages of the newest tags.
func (l DockerHubLister) ListTags(ctx context.Context, repository string) ([]string, error) {
	client := l.Client
	if client == nil {
		client = &http.Client{Timeout: tagFetchTimeout}
	}
	if !strings.Contains(repository, "/") {
		repository = "library/" + repository
	}
	next := fmt.Sprintf(dockerHubTagsURL, repository)
	var tags []string
	for page := 0; page < maxTagPages && next != ""; page++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, fmt.Errorf("dbupgrade: build tag request: %w", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("dbupgrade: list tags of %s: %w", repository, err)
		}
		var body hubTagsPage
		err = json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("dbupgrade: list tags of %s: registry returned %s", repository, resp.Status)
		}
		if err != nil {
			return nil, fmt.Errorf("dbupgrade: decode tags of %s: %w", repository, err)
		}
		for _, r := range body.Results {
			tags = append(tags, r.Name)
		}
		next = body.Next
		if next != "" && !strings.HasPrefix(next, "https://hub.docker.com/") {
			next = ""
		}
	}
	return tags, nil
}

// Refresher periodically merges newer registry tags into a Catalog. A failed
// refresh keeps the last good list, so the advisor never loses versions.
type Refresher struct {
	Catalog *Catalog
	Lister  TagLister
	Logger  *slog.Logger
}

// RefreshIntervalFromEnv returns the configured interval, 0 when disabled.
func RefreshIntervalFromEnv(logger *slog.Logger) time.Duration {
	raw := strings.TrimSpace(os.Getenv(EnvRefreshInterval))
	if raw == "" {
		return DefaultRefreshInterval
	}
	if raw == "0" || raw == "off" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		logger.Warn("invalid "+EnvRefreshInterval+", using the default", slog.String("value", raw))
		return DefaultRefreshInterval
	}
	return d
}

// Tick refreshes every engine whose image lives on Docker Hub.
func (r *Refresher) Tick(ctx context.Context) error {
	var errs []error
	for _, engine := range r.Catalog.Engines() {
		repo, ok := hubRepository(engine)
		if !ok {
			continue
		}
		ec, _ := r.Catalog.Engine(engine)
		if len(ec.Versions) == 0 {
			continue
		}
		tags, err := r.Lister.ListTags(ctx, repo)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		r.Catalog.SetDiscovered(engine, usableTags(ec, tags))
	}
	return errors.Join(errs...)
}

// Run refreshes once soon after start, then on interval until ctx is done.
func (r *Refresher) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if err := r.Tick(ctx); err != nil {
				r.Logger.Warn("dbupgrade: version refresh had errors, keeping the last good list", slog.String("error", err.Error()))
			}
			timer.Reset(interval)
		}
	}
}

func hubRepository(engine string) (string, bool) {
	ref := database.ImageRef(engine, "x")
	repo := strings.TrimSuffix(ref, ":x")
	first := strings.SplitN(repo, "/", 2)[0]
	if strings.ContainsAny(first, ".:") && strings.Contains(repo, "/") {
		return "", false
	}
	return repo, true
}

// usableTags keeps fully pinned plain tags on a line the curated catalog
// already knows, or newer than every curated version.
func usableTags(ec EngineCatalog, tags []string) []string {
	curated := parseAll(ec.Versions)
	depth, prefix := 0, ""
	lines := map[string]bool{}
	var newest version
	for i, v := range curated {
		if len(v.nums) > depth {
			depth = len(v.nums)
		}
		prefix = v.prefix
		lines[lineKey(ec.Levels, v)] = true
		if i == 0 || compareVersions(v, newest) > 0 {
			newest = v
		}
	}
	var out []string
	for _, t := range tags {
		v, ok := parseVersion(t)
		if !ok || v.suffix != "" || v.prefix != prefix || len(v.nums) != depth {
			continue
		}
		if lines[lineKey(ec.Levels, v)] || compareVersions(v, newest) > 0 {
			out = append(out, t)
		}
	}
	return out
}
