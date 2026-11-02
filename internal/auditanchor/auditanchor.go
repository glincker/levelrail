// Package auditanchor publishes the signed audit chain head to storage the
// operator controls, so a rewrite of the whole chain on this box is detectable.
package auditanchor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// Environment variables that turn the anchor on and tune it.
const (
	EnvDir      = "APP_AUDIT_ANCHOR_DIR"
	EnvURL      = "APP_AUDIT_ANCHOR_URL"
	EnvInterval = "APP_AUDIT_ANCHOR_INTERVAL"
	EnvTimeout  = "APP_AUDIT_ANCHOR_TIMEOUT"

	// FileName is the append-only export inside EnvDir.
	FileName = "audit-anchors.jsonl"

	defaultInterval = 15 * time.Minute
	defaultTimeout  = 10 * time.Second
)

// Config says where anchors go. Both sinks are optional; with neither set the
// feature is off.
type Config struct {
	Dir      string
	URL      string
	Interval time.Duration
	Timeout  time.Duration
}

// ConfigFromEnv reads the APP_AUDIT_ANCHOR_* variables; bad durations fall back to the defaults.
func ConfigFromEnv(lookup func(string) (string, bool)) Config {
	get := func(k string) string {
		v, _ := lookup(k)
		return strings.TrimSpace(v)
	}
	dur := func(k string, def time.Duration) time.Duration {
		if d, err := time.ParseDuration(get(k)); err == nil && d > 0 {
			return d
		}
		return def
	}
	return Config{Dir: get(EnvDir), URL: get(EnvURL), Interval: dur(EnvInterval, defaultInterval), Timeout: dur(EnvTimeout, defaultTimeout)}
}

// Enabled reports whether any sink is configured.
func (c Config) Enabled() bool { return c.Dir != "" || c.URL != "" }

// HeadSource yields the signed chain head; *store.DB satisfies it.
type HeadSource interface {
	LatestAuditAnchor(ctx context.Context) (store.AuditAnchor, bool, error)
}

// Publisher pushes new heads to the configured sinks.
type Publisher struct {
	cfg      Config
	src      HeadSource
	log      *slog.Logger
	client   *http.Client
	lastSeq  int64
	lastHash string
}

// New builds a Publisher; call Run to start it.
func New(cfg Config, src HeadSource, log *slog.Logger) *Publisher {
	return &Publisher{cfg: cfg, src: src, log: log, client: netguard.NewClient()}
}

// Run publishes on every interval until ctx ends. Sink errors are logged and
// retried next tick; they never reach the audit write path.
func (p *Publisher) Run(ctx context.Context) {
	if !p.cfg.Enabled() {
		return
	}
	p.Tick(ctx)
	t := time.NewTicker(p.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.Tick(ctx)
		}
	}
}

// Tick publishes the head once when it moved since the last success.
func (p *Publisher) Tick(ctx context.Context) {
	a, ok, err := p.src.LatestAuditAnchor(ctx)
	if err != nil {
		p.log.Warn("audit anchor: read chain head failed", slog.String("error", err.Error()))
		return
	}
	if !ok || (a.Seq == p.lastSeq && a.Hash == p.lastHash) {
		return
	}
	failed := false
	if p.cfg.Dir != "" {
		if err := appendFile(p.cfg.Dir, a); err != nil {
			failed = true
			p.log.Warn("audit anchor: file sink failed", slog.String("error", err.Error()))
		}
	}
	if p.cfg.URL != "" {
		if err := p.post(ctx, a); err != nil {
			failed = true
			p.log.Warn("audit anchor: webhook sink failed", slog.String("error", err.Error()))
		}
	}
	if !failed {
		p.lastSeq, p.lastHash = a.Seq, a.Hash
	}
}

func appendFile(dir string, a store.AuditAnchor) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create anchor dir: %w", err)
	}
	line, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("encode anchor: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, FileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // the operator names the anchor directory
	if err != nil {
		return fmt.Errorf("open anchor file: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return fmt.Errorf("write anchor file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close anchor file: %w", err)
	}
	return nil
}

func (p *Publisher) post(ctx context.Context, a store.AuditAnchor) error {
	if !strings.HasPrefix(strings.ToLower(p.cfg.URL), "https://") && !netguard.AllowPrivate() {
		return fmt.Errorf("anchor url must be https")
	}
	if err := netguard.ValidateURL(ctx, p.cfg.URL); err != nil {
		return fmt.Errorf("anchor url refused: %w", err)
	}
	body, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("encode anchor: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build anchor request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("post anchor: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("anchor sink answered %d", resp.StatusCode)
	}
	return nil
}
