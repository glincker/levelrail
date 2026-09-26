package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/netguard"
)

// Hub statuses reported by Preflight.
const (
	HFStatusOK          = "ok"
	HFStatusNotFound    = "not_found"
	HFStatusGated       = "gated"
	HFStatusRateLimited = "rate_limited"
	HFStatusUnavailable = "unavailable"
	HFStatusUnsupported = "unsupported"
)

// HFConfig tunes the Hugging Face Hub client.
type HFConfig struct {
	BaseURL          string
	Timeout          time.Duration
	CacheTTL         time.Duration
	MaxResponseBytes int64
	CacheEntries     int
}

// LoadHFConfig reads APP_HF_BASE_URL, APP_HF_TIMEOUT, APP_HF_CACHE_TTL and
// APP_HF_MAX_RESPONSE_BYTES, falling back to defaults on missing or bad values.
func LoadHFConfig() HFConfig {
	cfg := HFConfig{BaseURL: "https://huggingface.co", Timeout: 10 * time.Second, CacheTTL: 5 * time.Minute, MaxResponseBytes: 16 << 20, CacheEntries: 256}
	if v := strings.TrimRight(os.Getenv("APP_HF_BASE_URL"), "/"); v != "" {
		cfg.BaseURL = v
	}
	if d, err := time.ParseDuration(os.Getenv("APP_HF_TIMEOUT")); err == nil && d > 0 {
		cfg.Timeout = d
	}
	if d, err := time.ParseDuration(os.Getenv("APP_HF_CACHE_TTL")); err == nil && d >= 0 {
		cfg.CacheTTL = d
	}
	if n, err := strconv.ParseInt(os.Getenv("APP_HF_MAX_RESPONSE_BYTES"), 10, 64); err == nil && n > 0 {
		cfg.MaxResponseBytes = n
	}
	return cfg
}

// HFFile is one file of a Hub repository.
type HFFile struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// HFRepo is the Hub metadata Preflight needs.
type HFRepo struct {
	ID      string
	Gated   string
	Private bool
	License string
	Files   []HFFile
	// Access is "granted", "denied" or "unknown" for a gated repo, and
	// "not_required" otherwise.
	Access string
}

// HFError carries a non-success Hub outcome.
type HFError struct {
	Status     string
	Message    string
	RetryAfter time.Duration
}

func (e *HFError) Error() string { return "huggingface: " + e.Message }

// HFClient queries the Hub's public metadata API. It refuses internal
// addresses and never logs or returns the token it is given.
type HFClient struct {
	cfg    HFConfig
	http   *http.Client
	logger *slog.Logger
	now    func() time.Time

	mu    sync.Mutex
	cache map[string]hfCacheEntry
}

type hfCacheEntry struct {
	repo    *HFRepo
	err     *HFError
	expires time.Time
}

// NewHFClient builds a client. httpClient nil uses the SSRF-guarded client.
func NewHFClient(cfg HFConfig, httpClient *http.Client, logger *slog.Logger) *HFClient {
	if httpClient == nil {
		httpClient = netguard.NewClient()
	}
	c := *httpClient
	c.Timeout = cfg.Timeout
	if logger == nil {
		logger = slog.Default()
	}
	return &HFClient{cfg: cfg, http: &c, logger: logger, now: time.Now, cache: map[string]hfCacheEntry{}}
}

// CacheTTL is how long Repo answers are reused.
func (c *HFClient) CacheTTL() time.Duration { return c.cfg.CacheTTL }

func cacheKey(repo, token string) string {
	sum := sha256.Sum256([]byte(token))
	return repo + "|" + hex.EncodeToString(sum[:4])
}

type hubModelInfo struct {
	ID       string          `json:"id"`
	Private  bool            `json:"private"`
	Gated    json.RawMessage `json:"gated"`
	Tags     []string        `json:"tags"`
	CardData struct {
		License json.RawMessage `json:"license"`
	} `json:"cardData"`
	Siblings []struct {
		Name string `json:"rfilename"`
		Size *int64 `json:"size"`
	} `json:"siblings"`
}

// Repo returns the repository's metadata. Errors are *HFError. cached is
// true when the answer came from the short-lived cache.
func (c *HFClient) Repo(ctx context.Context, repo, token string) (r *HFRepo, cached bool, err error) {
	key := cacheKey(repo, token)
	if e, ok := c.lookup(key); ok {
		if e.err != nil {
			return nil, true, e.err
		}
		return e.repo, true, nil
	}
	r, herr := c.fetch(ctx, repo, token)
	if herr != nil {
		if herr.Status == HFStatusRateLimited || herr.Status == HFStatusUnavailable {
			c.logger.Warn("models: huggingface lookup failed", slog.String("repo", repo), slog.String("status", herr.Status), slog.String("reason", herr.Message))
		}
		if herr.Status == HFStatusNotFound {
			c.store(key, hfCacheEntry{err: herr})
		}
		return nil, false, herr
	}
	c.store(key, hfCacheEntry{repo: r})
	return r, false, nil
}

func (c *HFClient) lookup(key string) (hfCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || !c.now().Before(e.expires) {
		delete(c.cache, key)
		return hfCacheEntry{}, false
	}
	return e, true
}

func (c *HFClient) store(key string, e hfCacheEntry) {
	if c.cfg.CacheTTL <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.cache) >= c.cfg.CacheEntries {
		now := c.now()
		for k, v := range c.cache {
			if !now.Before(v.expires) {
				delete(c.cache, k)
			}
		}
		if len(c.cache) >= c.cfg.CacheEntries {
			c.cache = map[string]hfCacheEntry{}
		}
	}
	e.expires = c.now().Add(c.cfg.CacheTTL)
	c.cache[key] = e
}

func (c *HFClient) fetch(ctx context.Context, repo, token string) (*HFRepo, *HFError) {
	u := c.cfg.BaseURL + "/api/models/" + repo + "?blobs=true"
	resp, err := c.get(ctx, http.MethodGet, u, token, true)
	if err != nil {
		return nil, &HFError{Status: HFStatusUnavailable, Message: "could not reach Hugging Face: " + classifyNetErr(err)}
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, &HFError{Status: HFStatusRateLimited, Message: "Hugging Face is rate limiting requests", RetryAfter: retryAfter(resp)}
	case resp.StatusCode == http.StatusUnauthorized && token != "":
		return nil, &HFError{Status: HFStatusGated, Message: "Hugging Face rejected the token"}
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusNotFound:
		return nil, &HFError{Status: HFStatusNotFound, Message: "no such repository, or it is private (private repos need a token)"}
	case resp.StatusCode == http.StatusForbidden:
		return nil, &HFError{Status: HFStatusGated, Message: "access to this repository is forbidden for this token"}
	case resp.StatusCode >= 500:
		return nil, &HFError{Status: HFStatusUnavailable, Message: "Hugging Face returned an error (" + strconv.Itoa(resp.StatusCode) + ")"}
	case resp.StatusCode != http.StatusOK:
		return nil, &HFError{Status: HFStatusUnavailable, Message: "unexpected Hugging Face response (" + strconv.Itoa(resp.StatusCode) + ")"}
	}
	limited := &io.LimitedReader{R: resp.Body, N: c.cfg.MaxResponseBytes + 1}
	var info hubModelInfo
	if err := json.NewDecoder(limited).Decode(&info); err != nil {
		if limited.N <= 0 {
			return nil, &HFError{Status: HFStatusUnavailable, Message: "repository listing exceeds the response size cap (APP_HF_MAX_RESPONSE_BYTES)"}
		}
		return nil, &HFError{Status: HFStatusUnavailable, Message: "unreadable Hugging Face response"}
	}
	out := &HFRepo{ID: info.ID, Private: info.Private, Gated: parseGated(info.Gated), License: parseLicense(info), Access: "not_required"}
	if out.ID == "" {
		out.ID = repo
	}
	for _, s := range info.Siblings {
		f := HFFile{Name: s.Name}
		if s.Size != nil {
			f.Bytes = *s.Size
		}
		out.Files = append(out.Files, f)
	}
	if out.Gated != "" {
		out.Access = c.probeAccess(ctx, repo, token, out.Files)
	}
	return out, nil
}

// probeAccess asks the Hub whether the token may download a file of a
// gated repo. A redirect means yes, 401 or 403 means no.
func (c *HFClient) probeAccess(ctx context.Context, repo, token string, files []HFFile) string {
	if token == "" || len(files) == 0 {
		return "denied"
	}
	name := files[0].Name
	for _, f := range files {
		if f.Name == "config.json" {
			name = f.Name
			break
		}
	}
	resp, err := c.get(ctx, http.MethodHead, c.cfg.BaseURL+"/"+repo+"/resolve/main/"+name, token, false)
	if err != nil {
		return "unknown"
	}
	_ = resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return "denied"
	case resp.StatusCode < 400:
		return "granted"
	}
	return "unknown"
}

func (c *HFClient) get(ctx context.Context, method, u, token string, follow bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "levelrail-preflight")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := c.http
	if !follow {
		nc := *c.http
		nc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		client = &nc
	}
	return client.Do(req)
}

func classifyNetErr(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timed out"
	case errors.Is(err, netguard.ErrBlockedAddress):
		return "address blocked by the outbound network guard"
	case errors.Is(err, context.Canceled):
		return "request cancelled"
	}
	return "network error"
}

func retryAfter(resp *http.Response) time.Duration {
	if n, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && n > 0 && n < 3600 {
		return time.Duration(n) * time.Second
	}
	return 0
}

func parseGated(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		return s
	}
	var b bool
	if json.Unmarshal(raw, &b) == nil && b {
		return "auto"
	}
	return ""
}

func parseLicense(info hubModelInfo) string {
	var s string
	if json.Unmarshal(info.CardData.License, &s) == nil && s != "" {
		return s
	}
	var list []string
	if json.Unmarshal(info.CardData.License, &list) == nil && len(list) > 0 {
		return strings.Join(list, ", ")
	}
	for _, t := range info.Tags {
		if v, ok := strings.CutPrefix(t, "license:"); ok {
			return v
		}
	}
	return ""
}
