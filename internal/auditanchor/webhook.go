package auditanchor

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// Tunables for the webhook exporter; the destination itself is a runtime setting.
const (
	EnvQueue      = "APP_AUDIT_ANCHOR_QUEUE"
	EnvAttempts   = "APP_AUDIT_ANCHOR_ATTEMPTS"
	EnvBackoff    = "APP_AUDIT_ANCHOR_BACKOFF"
	EnvBackoffMax = "APP_AUDIT_ANCHOR_BACKOFF_MAX"

	// SignatureHeader carries "sha256=" plus the hex HMAC of the request body.
	SignatureHeader = "X-Levelrail-Signature"

	defaultQueue      = 64
	defaultAttempts   = 5
	defaultBackoff    = 2 * time.Second
	defaultBackoffMax = time.Minute
)

// Settings reads the live runtime settings; *runtimesettings.Service satisfies it.
type Settings interface {
	String(ctx context.Context, key string) string
}

// ExportConfig bounds the queue and the retry schedule.
type ExportConfig struct {
	Queue      int
	Attempts   int
	Backoff    time.Duration
	BackoffMax time.Duration
	Timeout    time.Duration
}

// ExportConfigFromEnv reads the APP_AUDIT_ANCHOR_* tunables; bad values fall back to defaults.
func ExportConfigFromEnv(lookup func(string) (string, bool)) ExportConfig {
	get := func(k string) string {
		v, _ := lookup(k)
		return strings.TrimSpace(v)
	}
	num := func(k string, def int) int {
		if n, err := strconv.Atoi(get(k)); err == nil && n > 0 {
			return n
		}
		return def
	}
	dur := func(k string, def time.Duration) time.Duration {
		if d, err := time.ParseDuration(get(k)); err == nil && d > 0 {
			return d
		}
		return def
	}
	return ExportConfig{
		Queue: num(EnvQueue, defaultQueue), Attempts: num(EnvAttempts, defaultAttempts),
		Backoff: dur(EnvBackoff, defaultBackoff), BackoffMax: dur(EnvBackoffMax, defaultBackoffMax),
		Timeout: dur(EnvTimeout, defaultTimeout),
	}
}

// Status is the operator view of the webhook export. It holds no secret and no URL.
type Status struct {
	Enabled         bool   `json:"enabled"`
	LastExportedSeq int64  `json:"last_exported_seq"`
	LastExportedAt  string `json:"last_exported_at,omitempty"`
	LastError       string `json:"last_error,omitempty"`
	LastErrorAt     string `json:"last_error_at,omitempty"`
	Exported        int64  `json:"exported"`
	Failed          int64  `json:"failed"`
	Dropped         int64  `json:"dropped"`
	Queued          int    `json:"queued"`
}

// Exporter sends each new checkpoint to the configured webhook from a bounded
// queue, so a slow or dead sink never reaches the audit write path.
type Exporter struct {
	cfg      ExportConfig
	interval time.Duration
	src      HeadSource
	set      Settings
	log      *slog.Logger
	client   *http.Client
	queue    chan store.AuditAnchor
	sleep    func(context.Context, time.Duration)

	mu       sync.Mutex
	queuedTo store.AuditAnchor
	st       Status
}

// NewExporter builds an Exporter; call Run to start it.
func NewExporter(cfg ExportConfig, interval time.Duration, src HeadSource, set Settings, log *slog.Logger) *Exporter {
	if cfg.Queue < 1 {
		cfg.Queue = defaultQueue
	}
	if cfg.Attempts < 1 {
		cfg.Attempts = defaultAttempts
	}
	if interval <= 0 {
		interval = defaultInterval
	}
	return &Exporter{
		cfg: cfg, interval: interval, src: src, set: set, log: log, client: netguard.NewClient(),
		queue: make(chan store.AuditAnchor, cfg.Queue), sleep: sleepCtx,
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// Status returns a snapshot for the API and CLI.
func (e *Exporter) Status(ctx context.Context) Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.st
	s.Enabled = e.url(ctx) != ""
	s.Queued = len(e.queue)
	return s
}

func (e *Exporter) url(ctx context.Context) string {
	return strings.TrimSpace(e.set.String(ctx, runtimesettings.KeyAuditAnchorWebhookURL))
}

// Run polls the chain head and drains the queue until ctx ends.
func (e *Exporter) Run(ctx context.Context) {
	go e.drain(ctx)
	e.Tick(ctx)
	t := time.NewTicker(e.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.Tick(ctx)
		}
	}
}

// Tick queues the head when it moved. With no URL set it makes no request and queues nothing.
func (e *Exporter) Tick(ctx context.Context) {
	if e.url(ctx) == "" {
		return
	}
	a, ok, err := e.src.LatestAuditAnchor(ctx)
	if err != nil {
		e.log.Warn("audit anchor export: read chain head failed", slog.String("error", err.Error()))
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !ok || (a.Seq == e.queuedTo.Seq && a.Hash == e.queuedTo.Hash) {
		return
	}
	select {
	case e.queue <- a:
		e.queuedTo = a
	default:
		// Full: keep queuedTo unchanged so the newest head is offered again next tick.
		e.st.Dropped++
		e.log.Warn("audit anchor export: queue full, checkpoint deferred", slog.Int64("seq", a.Seq))
	}
}

func (e *Exporter) drain(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case a := <-e.queue:
			e.deliver(ctx, a)
		}
	}
}

// deliver retries one checkpoint with doubling backoff, then gives up and counts it.
func (e *Exporter) deliver(ctx context.Context, a store.AuditAnchor) {
	delay := e.cfg.Backoff
	var err error
	for attempt := 1; attempt <= e.cfg.Attempts; attempt++ {
		if e.url(ctx) == "" {
			return
		}
		if err = e.send(ctx, a); err == nil {
			e.mu.Lock()
			e.st.Exported++
			e.st.LastExportedSeq = a.Seq
			e.st.LastExportedAt = time.Now().UTC().Format(time.RFC3339)
			e.mu.Unlock()
			return
		}
		e.log.Warn("audit anchor export: attempt failed", slog.Int64("seq", a.Seq), slog.Int("attempt", attempt), slog.String("error", err.Error()))
		if attempt < e.cfg.Attempts {
			e.sleep(ctx, delay)
			if ctx.Err() != nil {
				return
			}
			if delay *= 2; e.cfg.BackoffMax > 0 && delay > e.cfg.BackoffMax {
				delay = e.cfg.BackoffMax
			}
		}
	}
	e.mu.Lock()
	e.st.Failed++
	e.st.LastError = err.Error()
	e.st.LastErrorAt = time.Now().UTC().Format(time.RFC3339)
	e.mu.Unlock()
}

// Sign returns the header value for body under secret.
func Sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func (e *Exporter) send(ctx context.Context, a store.AuditAnchor) error {
	target := e.url(ctx)
	if !strings.HasPrefix(strings.ToLower(target), "https://") && !netguard.AllowPrivate() {
		return fmt.Errorf("anchor webhook url must be https")
	}
	if err := netguard.ValidateURL(ctx, target); err != nil {
		return fmt.Errorf("anchor webhook url refused: %w", err)
	}
	body, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("encode anchor: %w", err)
	}
	timeout := e.cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build anchor request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if secret := e.set.String(ctx, runtimesettings.KeyAuditAnchorWebhookSecret); secret != "" {
		req.Header.Set(SignatureHeader, Sign(secret, body))
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("post anchor: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("anchor webhook answered %d", resp.StatusCode)
	}
	return nil
}
