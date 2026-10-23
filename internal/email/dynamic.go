package email

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// ConfigLoader resolves the current Config on every call, never cached:
// settings can change at runtime.
type ConfigLoader func(ctx context.Context) (Config, error)

// Delivery is one send outcome handed to a Recorder.
type Delivery struct {
	At        time.Time
	Provider  string
	To        string
	Kind      string
	Status    string // DeliverySent or DeliveryFailed
	MessageID string
	Error     string
	ErrorKind FailureKind
	Attempts  int
	// ProviderID is the provider row that handled the attempt; FailoverFrom is the one it replaced, if any.
	ProviderID   string
	FailoverFrom string
}

// Delivery statuses.
const (
	DeliverySent    = "sent"
	DeliveryFailed  = "failed"
	DeliveryBounced = "bounced"
)

// Recorder persists delivery outcomes and answers the sent-count questions the rate caps need.
type Recorder interface {
	RecordDelivery(ctx context.Context, d Delivery) error
	CountSent(ctx context.Context, since time.Time) (int, error)
}

// Result describes a successful send.
type Result struct {
	Provider  string
	MessageID string
	Attempts  int
}

type ctxKey int

const (
	kindKey ctxKey = iota
	noRetryKey
	noFailoverKey
	onlyProviderKey
)

// WithKind tags sends made with ctx (for example "password_reset") in the delivery log.
func WithKind(ctx context.Context, kind string) context.Context {
	return context.WithValue(ctx, kindKey, kind)
}

// WithoutRetry disables retries for sends made with ctx, for interactive test sends.
// It also disables failover: a test send must report on the provider it was aimed at.
func WithoutRetry(ctx context.Context) context.Context {
	return context.WithValue(ctx, noRetryKey, true)
}

// WithoutFailover keeps sends made with ctx on the primary provider even when it fails.
func WithoutFailover(ctx context.Context) context.Context {
	return context.WithValue(ctx, noFailoverKey, true)
}

// WithOnlyProvider aims sends made with ctx at one provider id, with no failover. A ProvidersLoader should honor it
// even for a disabled provider, so a test send can prove a provider before it is enabled.
func WithOnlyProvider(ctx context.Context, id string) context.Context {
	return context.WithValue(WithoutFailover(ctx), onlyProviderKey, id)
}

// OnlyProviderID returns the id set by WithOnlyProvider, or "".
func OnlyProviderID(ctx context.Context) string {
	id, _ := ctx.Value(onlyProviderKey).(string)
	return id
}

// ProviderHealth persists the last outcome per provider. Implementations are best effort.
type ProviderHealth interface {
	RecordProviderResult(ctx context.Context, providerID string, ok bool, kind FailureKind, at time.Time) error
}

// DynamicSender adapts a ConfigLoader into a Sender, so a settings
// change takes effect on the very next send with no restart required.
type DynamicSender struct {
	load        ConfigLoader
	recorder    Recorder
	maxAttempts int
	baseDelay   time.Duration

	loadProviders ProvidersLoader
	breaker       *breaker
	health        ProviderHealth
	logger        *slog.Logger
}

// NewDynamicSender builds a DynamicSender. load must not be nil.
func NewDynamicSender(load ConfigLoader) *DynamicSender {
	return &DynamicSender{
		load: load, maxAttempts: 3, baseDelay: 500 * time.Millisecond,
		breaker: newBreaker(DefaultFailoverThreshold, DefaultFailoverCooldown),
	}
}

// WithProviders makes sends use an ordered provider list (primary first, optional backup) instead of the single ConfigLoader.
func (d *DynamicSender) WithProviders(load ProvidersLoader) *DynamicSender {
	d.loadProviders = load
	return d
}

// WithFailover sets how many consecutive failover-class failures open a provider's circuit and how long it stays open.
func (d *DynamicSender) WithFailover(threshold int, cooldown time.Duration) *DynamicSender {
	if threshold < 1 {
		threshold = DefaultFailoverThreshold
	}
	if cooldown <= 0 {
		cooldown = DefaultFailoverCooldown
	}
	d.breaker = newBreaker(threshold, cooldown)
	return d
}

// WithProviderHealth persists last_ok and last error kind per provider after sends.
func (d *DynamicSender) WithProviderHealth(h ProviderHealth) *DynamicSender {
	d.health = h
	return d
}

// WithLogger enables structured failover logging; without it nothing is logged.
func (d *DynamicSender) WithLogger(l *slog.Logger) *DynamicSender {
	d.logger = l
	return d
}

func (d *DynamicSender) providers(ctx context.Context) ([]ProviderConfig, error) {
	if d.loadProviders != nil {
		provs, err := d.loadProviders(ctx)
		if err != nil {
			return nil, err
		}
		if only := OnlyProviderID(ctx); only != "" {
			for _, p := range provs {
				if p.ID == only {
					return []ProviderConfig{p}, nil
				}
			}
			return nil, fmt.Errorf("email provider %q is not available", only)
		}
		return provs, nil
	}
	cfg, err := d.load(ctx)
	if err != nil {
		return nil, err
	}
	return []ProviderConfig{{Config: cfg}}, nil
}

// WithRecorder logs every send outcome and enforces the configured send caps.
func (d *DynamicSender) WithRecorder(r Recorder) *DynamicSender {
	d.recorder = r
	return d
}

// WithRetry sets how many attempts a transient provider failure gets and the first backoff delay.
func (d *DynamicSender) WithRetry(attempts int, base time.Duration) *DynamicSender {
	if attempts >= 1 {
		d.maxAttempts = attempts
	}
	if base > 0 {
		d.baseDelay = base
	}
	return d
}

// Send implements Sender. It rejects a CR or LF in to or subject before
// any backend runs, since either could start an injected header.
func (d *DynamicSender) Send(ctx context.Context, to, subject, body string) error {
	_, err := d.SendDetailed(ctx, to, subject, body)
	return err
}

// SendDetailed is Send plus the provider and message id of a successful send.
func (d *DynamicSender) SendDetailed(ctx context.Context, to, subject, body string) (Result, error) {
	if err := rejectHeaderInjection(to, subject); err != nil {
		return Result{}, err
	}
	return d.run(ctx, to, func(s Sender) (string, error) { return SendWithID(ctx, s, to, subject, body) })
}

// SendMessage implements RichSender through the current settings, with the same retries, caps and delivery log as Send.
func (d *DynamicSender) SendMessage(ctx context.Context, m Message) error {
	_, err := d.SendMessageDetailed(ctx, m)
	return err
}

// SendMessageDetailed is SendMessage plus the provider and message id of a successful send.
func (d *DynamicSender) SendMessageDetailed(ctx context.Context, m Message) (Result, error) {
	if err := validateMessage(m); err != nil {
		return Result{}, err
	}
	return d.run(ctx, m.To, func(s Sender) (string, error) { return sendMessageWithID(ctx, s, m) })
}

func (d *DynamicSender) run(ctx context.Context, to string, send func(Sender) (string, error)) (Result, error) {
	provs, err := d.providers(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("email: load config: %w", err)
	}
	if len(provs) == 0 {
		provs = []ProviderConfig{{}}
	}
	primary := provs[0]
	sender, err := NewSender(primary.Config)
	if err != nil {
		return Result{}, err
	}
	kind, _ := ctx.Value(kindKey).(string)

	if err := d.checkLimits(ctx, primary.Config.Limits); err != nil {
		d.record(ctx, Delivery{Provider: string(primary.Config.Backend), ProviderID: primary.ID, To: to, Kind: kind, Status: DeliveryFailed, Error: err.Error(), ErrorKind: FailRateLimited})
		return Result{}, &SendError{Kind: FailRateLimited, Attempts: 0, Err: err}
	}

	attempts := d.maxAttempts
	noRetry, _ := ctx.Value(noRetryKey).(bool)
	if noRetry {
		attempts = 1
	}
	noFailover, _ := ctx.Value(noFailoverKey).(bool)
	failover := len(provs) > 1 && !noRetry && !noFailover

	order := []int{0}
	failedFrom := ""
	if failover {
		order = order[:0]
		for i, p := range provs {
			if d.breaker.allow(p.ID) {
				order = append(order, i)
			} else {
				d.logFailover(slog.LevelWarn, "email provider circuit open", p, "", 0)
				if failedFrom == "" {
					failedFrom = p.ID
				}
			}
		}
		if len(order) == 0 {
			order, failedFrom = []int{0}, ""
		}
	}

	var errs []error
	var firstKind FailureKind
	total := 0
	for pos, i := range order {
		p := provs[i]
		s := sender
		if i != 0 {
			if s, err = NewSender(p.Config); err != nil {
				d.breaker.report(p.ID, outcomeNeutral)
				d.logFailover(slog.LevelWarn, "email backup provider unusable", p, "", 0)
				errs = append(errs, fmt.Errorf("%s: %w", p.ID, err))
				continue
			}
		}
		id, n, serr := d.attempt(ctx, s, send, attempts)
		total += n
		backend := string(p.Config.Backend)
		if serr == nil {
			if failover {
				d.breaker.report(p.ID, outcomeOK)
			}
			d.record(ctx, Delivery{Provider: backend, ProviderID: p.ID, FailoverFrom: failedFrom, To: to, Kind: kind, Status: DeliverySent, MessageID: id, Attempts: n})
			d.persistHealth(ctx, p.ID, true, "")
			if failedFrom != "" {
				d.logFailover(slog.LevelInfo, "email sent through backup provider", p, "", n)
			}
			return Result{Provider: backend, MessageID: id, Attempts: total}, nil
		}
		k := Classify(serr)
		eligible := failover && ctx.Err() == nil && FailoverEligible(serr)
		if firstKind == "" {
			firstKind = k
		}
		d.record(ctx, Delivery{Provider: backend, ProviderID: p.ID, FailoverFrom: failedFrom, To: to, Kind: kind, Status: DeliveryFailed, Error: errorText(serr, configSecrets(p.Config)), ErrorKind: k, Attempts: n})
		errs = append(errs, serr)
		switch {
		case eligible:
			d.breaker.report(p.ID, outcomeFail)
			d.persistHealth(ctx, p.ID, false, k)
		case failover && ctx.Err() == nil && k != FailRateLimited:
			d.breaker.report(p.ID, outcomeOK) // the provider answered; the refusal was about the recipient or content
		case failover:
			d.breaker.report(p.ID, outcomeNeutral)
		}
		if !eligible {
			break
		}
		failedFrom = p.ID
		if pos+1 < len(order) {
			d.logFailover(slog.LevelWarn, "email failing over", p, provs[order[pos+1]].ID, n)
		}
	}
	if len(errs) == 1 {
		return Result{}, &SendError{Kind: firstKind, Attempts: total, Err: errs[0]}
	}
	return Result{}, &SendError{Kind: firstKind, Attempts: total, Err: fmt.Errorf("email: every provider failed: %w", errors.Join(errs...))}
}

// attempt runs one provider's send with its own transient-error retries.
func (d *DynamicSender) attempt(ctx context.Context, sender Sender, send func(Sender) (string, error), attempts int) (string, int, error) {
	var id string
	var err error
	n := 0
	delay := d.baseDelay
	for {
		n++
		id, err = send(sender)
		if err == nil || n >= attempts || !IsTransient(err) {
			break
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
		if ctx.Err() != nil {
			break
		}
		delay *= 2
	}
	return id, n, err
}

func (d *DynamicSender) logFailover(level slog.Level, msg string, p ProviderConfig, failedOverTo string, attempt int) {
	if d.logger == nil {
		return
	}
	attrs := []slog.Attr{slog.String("provider_id", p.ID), slog.String("backend", string(p.Config.Backend)), slog.Int("attempt", attempt)}
	if failedOverTo != "" {
		attrs = append(attrs, slog.String("failed_over_to", failedOverTo))
	}
	d.logger.LogAttrs(context.Background(), level, msg, attrs...)
}

func (d *DynamicSender) persistHealth(ctx context.Context, providerID string, ok bool, kind FailureKind) {
	if d.health == nil || providerID == "" {
		return
	}
	hctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := d.health.RecordProviderResult(hctx, providerID, ok, kind, time.Now().UTC()); err != nil && d.logger != nil {
		d.logger.Warn("email provider health not saved", slog.String("provider_id", providerID))
	}
}

func (d *DynamicSender) record(ctx context.Context, del Delivery) {
	if d.recorder == nil {
		return
	}
	if del.At.IsZero() {
		del.At = time.Now().UTC()
	}
	_ = d.recorder.RecordDelivery(context.WithoutCancel(ctx), del)
}

func (d *DynamicSender) checkLimits(ctx context.Context, l Limits) error {
	if d.recorder == nil || (l.PerMinute <= 0 && l.PerDay <= 0) {
		return nil
	}
	now := time.Now().UTC()
	if l.PerMinute > 0 {
		n, err := d.recorder.CountSent(ctx, now.Add(-time.Minute))
		if err == nil && n >= l.PerMinute {
			return fmt.Errorf("%w: %d sends in the last minute", ErrRateLimited, n)
		}
	}
	if l.PerDay > 0 {
		n, err := d.recorder.CountSent(ctx, now.Add(-24*time.Hour))
		if err == nil && n >= l.PerDay {
			return fmt.Errorf("%w: %d sends in the last 24 hours", ErrRateLimited, n)
		}
	}
	return nil
}

// Configured reports whether the current settings name a backend that can
// send, so callers can tell "no mail server set up" from a failed send.
func (d *DynamicSender) Configured(ctx context.Context) bool {
	provs, err := d.providers(ctx)
	if err != nil || len(provs) == 0 {
		return false
	}
	_, err = NewSender(provs[0].Config)
	return err == nil
}

// rejectHeaderInjection reports an error if to or subject carries a CR
// or LF.
func rejectHeaderInjection(to, subject string) error {
	if strings.ContainsAny(to, "\r\n") {
		return fmt.Errorf("email: to address contains a newline")
	}
	if strings.ContainsAny(subject, "\r\n") {
		return fmt.Errorf("email: subject contains a newline")
	}
	return nil
}
