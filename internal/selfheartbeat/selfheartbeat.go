// Package selfheartbeat sends an outbound ping so an external monitor can page
// when the control plane itself goes silent.
package selfheartbeat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// Env names for the fixed bounds; the URL and interval are live runtime settings.
const (
	EnvTimeout  = "APP_SELF_HEARTBEAT_TIMEOUT"
	EnvLogEvery = "APP_SELF_HEARTBEAT_LOG_EVERY"

	defaultTimeout  = 10 * time.Second
	defaultLogEvery = 10 * time.Minute
	maxTimeout      = 30 * time.Second
	defaultInterval = 60 * time.Second
	maxRedirects    = 3
	// pollEvery is how often Run re-reads the settings, so a saved URL or a lowered interval arms within seconds.
	pollEvery = 5 * time.Second
)

// Settings reads live runtime settings; *runtimesettings.Service satisfies it.
type Settings interface {
	String(ctx context.Context, key string) string
	Int(ctx context.Context, key string) int
}

// Config bounds one request and the failure log rate.
type Config struct {
	Timeout  time.Duration
	LogEvery time.Duration
}

// ConfigFromEnv reads the tunables; bad values fall back to defaults.
func ConfigFromEnv(lookup func(string) (string, bool)) Config {
	dur := func(k string, def time.Duration) time.Duration {
		v, _ := lookup(k)
		if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil && d > 0 {
			return min(d, maxTimeout)
		}
		return def
	}
	return Config{Timeout: dur(EnvTimeout, defaultTimeout), LogEvery: dur(EnvLogEvery, defaultLogEvery)}
}

// Pinger sends the heartbeat GET.
type Pinger struct {
	cfg      Config
	set      Settings
	log      *slog.Logger
	client   *http.Client
	validate func(ctx context.Context, raw string) error
	now      func() time.Time

	failing   bool
	lastPing  time.Time
	lastLogAt time.Time
}

// New builds a Pinger on the SSRF-guarded client; call Run to start it.
func New(cfg Config, set Settings, log *slog.Logger) *Pinger {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.LogEvery <= 0 {
		cfg.LogEvery = defaultLogEvery
	}
	c := netguard.NewClient()
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "https" && !netguard.AllowPrivate() {
			return errors.New("redirect to a non-https url refused")
		}
		return nil
	}
	return &Pinger{cfg: cfg, set: set, log: log, client: c, validate: netguard.ValidateURL, now: time.Now}
}

// Interval is the live ping interval, clamped to the setting's range.
func (p *Pinger) Interval(ctx context.Context) time.Duration {
	n := p.set.Int(ctx, runtimesettings.KeyAlertingSelfHeartbeatInterval)
	if n < runtimesettings.SelfHeartbeatMinInterval || n > runtimesettings.SelfHeartbeatMaxInterval {
		return defaultInterval
	}
	return time.Duration(n) * time.Second
}

// Run polls the settings every few seconds and pings once the live interval has elapsed.
func (p *Pinger) Run(ctx context.Context) {
	t := time.NewTicker(pollEvery)
	defer t.Stop()
	for {
		p.Poll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Poll pings when the interval has elapsed since the last ping; with no URL it does nothing.
func (p *Pinger) Poll(ctx context.Context) {
	if p.now().Sub(p.lastPing) < p.Interval(ctx) {
		return
	}
	if strings.TrimSpace(p.set.String(ctx, runtimesettings.KeyAlertingSelfHeartbeatURL)) == "" {
		p.failing = false
		return
	}
	p.lastPing = p.now()
	p.Tick(ctx)
}

// Tick sends one ping. With no URL set it makes no request.
func (p *Pinger) Tick(ctx context.Context) {
	target := strings.TrimSpace(p.set.String(ctx, runtimesettings.KeyAlertingSelfHeartbeatURL))
	if target == "" {
		p.failing = false
		return
	}
	err := p.ping(ctx, target)
	if err == nil {
		if p.failing {
			p.log.Info("self heartbeat recovered", slog.String("host", hostOf(target)))
		}
		p.failing = false
		return
	}
	// Log the first failure and then at most once per window, so a dead monitor cannot flood the log.
	if !p.failing || p.now().Sub(p.lastLogAt) >= p.cfg.LogEvery {
		p.log.Warn("self heartbeat ping failed", slog.String("host", hostOf(target)), slog.String("error", err.Error()))
		p.lastLogAt = p.now()
	}
	p.failing = true
}

func (p *Pinger) ping(ctx context.Context, target string) error {
	if !strings.HasPrefix(strings.ToLower(target), "https://") && !netguard.AllowPrivate() {
		return errors.New("heartbeat url must be https")
	}
	if err := p.validate(ctx, target); err != nil {
		return errors.New("heartbeat url refused")
	}
	rctx, cancel := context.WithTimeout(ctx, min(p.cfg.Timeout, p.Interval(ctx)))
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, target, nil)
	if err != nil {
		return errors.New("build heartbeat request")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		// url.Error embeds the full URL, which may carry a token, so keep only the cause.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("ping: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode/100 != 2 {
		return errors.New("monitor answered " + strconv.Itoa(resp.StatusCode))
	}
	return nil
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "invalid"
	}
	return u.Hostname()
}
