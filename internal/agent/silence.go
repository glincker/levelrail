package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/heartbeat"
)

// Env knobs for the control plane silence watcher, all durations except the jitter percent.
const (
	EnvSilenceGrace      = "APP_AGENT_CP_SILENCE_GRACE"
	EnvSilenceInterval   = "APP_AGENT_CP_SILENCE_INTERVAL"
	EnvSilenceMaxBackoff = "APP_AGENT_CP_SILENCE_MAX_BACKOFF"
	EnvSilenceCheck      = "APP_AGENT_CP_SILENCE_CHECK"
	EnvSilenceTimeout    = "APP_AGENT_CP_SILENCE_TIMEOUT"
	EnvSilenceJitterPct  = "APP_AGENT_CP_SILENCE_JITTER_PCT"

	defaultSilenceGrace      = 5 * time.Minute
	defaultSilenceInterval   = time.Minute
	defaultSilenceMaxBackoff = 15 * time.Minute
	defaultSilenceCheck      = 10 * time.Second
	defaultSilenceTimeout    = 10 * time.Second
	defaultSilenceJitterPct  = 20
)

// SilenceConfig tunes the watcher; zero fields fall back to the defaults.
type SilenceConfig struct {
	Grace, Interval, MaxBackoff, Check, Timeout time.Duration
	JitterPct                                   int
	AllowPrivate                                bool
}

// SilenceConfigFromEnv reads the watcher env; a bad value is an error so a typo does not silently disable alerting.
func SilenceConfigFromEnv(lookup func(string) (string, bool)) (SilenceConfig, error) {
	c := SilenceConfig{Grace: defaultSilenceGrace, Interval: defaultSilenceInterval, MaxBackoff: defaultSilenceMaxBackoff,
		Check: defaultSilenceCheck, Timeout: defaultSilenceTimeout, JitterPct: defaultSilenceJitterPct}
	get := func(k string) string { v, _ := lookup(k); return strings.TrimSpace(v) }
	for _, d := range []struct {
		key string
		dst *time.Duration
	}{{EnvSilenceGrace, &c.Grace}, {EnvSilenceInterval, &c.Interval}, {EnvSilenceMaxBackoff, &c.MaxBackoff},
		{EnvSilenceCheck, &c.Check}, {EnvSilenceTimeout, &c.Timeout}} {
		v := get(d.key)
		if v == "" {
			continue
		}
		p, err := time.ParseDuration(v)
		if err != nil || p <= 0 {
			return SilenceConfig{}, fmt.Errorf("%s must be a positive duration", d.key)
		}
		*d.dst = p
	}
	if v := get(EnvSilenceJitterPct); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 0 || p > 100 {
			return SilenceConfig{}, fmt.Errorf("%s must be an integer from 0 to 100", EnvSilenceJitterPct)
		}
		c.JitterPct = p
	}
	switch strings.ToLower(get(heartbeat.EnvAllowPrivate)) {
	case "1", "true", "yes", "on":
		c.AllowPrivate = true
	}
	return c, nil
}

// SilenceWatcher pings an off-host URL while this agent has had no contact with the control plane for longer than the grace.
type SilenceWatcher struct {
	cfg    SilenceConfig
	node   string
	logger *slog.Logger
	client *http.Client
	now    func() time.Time
	rnd    func(int64) int64

	mu          sync.Mutex
	url         string
	connected   bool
	lastContact time.Time
}

// NewSilenceWatcher builds a watcher; contact is assumed fresh at start so a booting agent does not alert before it dials.
func NewSilenceWatcher(cfg SilenceConfig, node string, logger *slog.Logger, rnd func(int64) int64) *SilenceWatcher {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &SilenceWatcher{cfg: cfg, node: node, logger: logger, rnd: rnd, now: time.Now,
		client: heartbeat.NewClient(cfg.AllowPrivate, cfg.Timeout), lastContact: time.Now()}
}

// SetURL stores the configured URL; an empty or invalid one disables the watcher.
func (w *SilenceWatcher) SetURL(raw string) {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		if _, err := heartbeat.ValidateURL(raw, w.cfg.AllowPrivate); err != nil {
			w.logger.Warn("agent: silence watcher: rejected URL from control plane", slog.String("error", err.Error()))
			raw = ""
		}
	}
	w.mu.Lock()
	w.url = raw
	w.mu.Unlock()
}

// SetConnected marks a session as up or down; the silence clock starts when it goes down.
func (w *SilenceWatcher) SetConnected(up bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.connected = up
	w.lastContact = w.now()
}

// silentFor reports how long contact has been missing and the URL to ping, or 0 when in contact.
func (w *SilenceWatcher) silentFor() (time.Duration, string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.connected {
		return 0, w.url
	}
	return w.now().Sub(w.lastContact), w.url
}

func (w *SilenceWatcher) jitter(d time.Duration) time.Duration {
	if w.cfg.JitterPct <= 0 || w.rnd == nil {
		return d
	}
	return d + time.Duration(w.rnd(int64(d)*int64(w.cfg.JitterPct)/100+1))
}

// Run checks every Check and pings with jittered interval while silent, backing off on failed pings, until ctx ends.
func (w *SilenceWatcher) Run(ctx context.Context) {
	wait := w.cfg.Check
	backoff := w.cfg.Interval
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		silent, raw := w.silentFor()
		if raw == "" || silent < w.cfg.Grace {
			wait, backoff = w.cfg.Check, w.cfg.Interval
			continue
		}
		if err := w.ping(ctx, raw, silent); err != nil {
			w.logger.Warn("agent: silence ping failed", slog.String("host", heartbeat.HostOf(raw)), slog.String("error", err.Error()))
			wait = w.jitter(backoff)
			backoff = min(backoff*2, w.cfg.MaxBackoff)
			continue
		}
		w.logger.Warn("agent: control plane silent, reported off-host", slog.String("host", heartbeat.HostOf(raw)), slog.Duration("silent_for", silent.Round(time.Second)))
		wait, backoff = w.jitter(w.cfg.Interval), w.cfg.Interval
	}
}

func (w *SilenceWatcher) ping(ctx context.Context, raw string, silent time.Duration) error {
	body, _ := json.Marshal(map[string]any{
		"status": "control_plane_silent", "node": w.node, "silent_seconds": int64(silent.Seconds()),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, raw, bytes.NewReader(body))
	if err != nil {
		return heartbeat.Scrub(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		return heartbeat.Scrub(err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("monitor answered HTTP %d", resp.StatusCode)
	}
	return nil
}

type silenceCtxKey struct{}

func withSilence(ctx context.Context, w *SilenceWatcher) context.Context {
	return context.WithValue(ctx, silenceCtxKey{}, w)
}

func silenceFrom(ctx context.Context) *SilenceWatcher {
	w, _ := ctx.Value(silenceCtxKey{}).(*SilenceWatcher)
	return w
}

// WithSilenceWatcher feeds w the session state and the URL the control plane pushes.
func WithSilenceWatcher(w *SilenceWatcher) SessionOption {
	return func(c *sessionConfig) { c.silence = w }
}
