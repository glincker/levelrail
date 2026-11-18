package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	demoStarsRefresh = 6 * time.Hour
	// A failed fetch retries sooner so a transient outage does not hide the count for hours.
	demoStarsRetry   = 15 * time.Minute
	demoStarsTimeout = 5 * time.Second
	demoStarsDefault = "glincker/levelrail"
	demoStarsAPIBase = "https://api.github.com"
)

var errDemoStarsStatus = errors.New("demo stars: unusable response")

var demoStarsRepoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// demoStars caches the repository star count. Get never blocks: a stale value
// is served while one background goroutine refreshes it.
type demoStars struct {
	url    string
	client *http.Client
	now    func() time.Time

	mu       sync.Mutex
	value    int
	ok       bool
	nextAt   time.Time
	inflight bool
}

// demoStarsFromEnv returns nil when the fetch is disabled or the repo is malformed,
// so a locked-down host that blocks outbound traffic simply omits the count.
func demoStarsFromEnv(getenv func(string) string, lookup func(string) (string, bool)) *demoStars {
	if v, err := strconv.ParseBool(strings.TrimSpace(getenv("APP_DEMO_STARS_DISABLE"))); err == nil && v {
		return nil
	}
	repo := demoStarsDefault
	if v, set := lookup("APP_DEMO_STARS_REPO"); set {
		repo = strings.TrimSpace(v)
	}
	if !demoStarsRepoPattern.MatchString(repo) {
		return nil
	}
	return &demoStars{
		url:    demoStarsAPIBase + "/repos/" + repo,
		client: &http.Client{Timeout: demoStarsTimeout},
		now:    time.Now,
	}
}

func demoStarsFromProcessEnv() *demoStars {
	return demoStarsFromEnv(os.Getenv, os.LookupEnv)
}

// Get returns the last good count and starts a refresh when it is due.
func (s *demoStars) Get() (int, bool) {
	if s == nil {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.inflight && !s.now().Before(s.nextAt) {
		s.inflight = true
		go s.refresh()
	}
	return s.value, s.ok
}

func (s *demoStars) refresh() {
	n, err := s.fetch()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inflight = false
	if err != nil {
		s.nextAt = s.now().Add(demoStarsRetry)
		return
	}
	s.value, s.ok = n, true
	s.nextAt = s.now().Add(demoStarsRefresh)
}

func (s *demoStars) fetch() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), demoStarsTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "levelrail-demo")
	res, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return 0, errDemoStarsStatus
	}
	var body struct {
		Stars *int `json:"stargazers_count"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, res.Body, 1<<20)).Decode(&body); err != nil || body.Stars == nil || *body.Stars < 0 {
		return 0, errDemoStarsStatus
	}
	return *body.Stars, nil
}
