package email

import (
	"context"
	"fmt"
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
)

// WithKind tags sends made with ctx (for example "password_reset") in the delivery log.
func WithKind(ctx context.Context, kind string) context.Context {
	return context.WithValue(ctx, kindKey, kind)
}

// WithoutRetry disables retries for sends made with ctx, for interactive test sends.
func WithoutRetry(ctx context.Context) context.Context {
	return context.WithValue(ctx, noRetryKey, true)
}

// DynamicSender adapts a ConfigLoader into a Sender, so a settings
// change takes effect on the very next send with no restart required.
type DynamicSender struct {
	load        ConfigLoader
	recorder    Recorder
	maxAttempts int
	baseDelay   time.Duration
}

// NewDynamicSender builds a DynamicSender. load must not be nil.
func NewDynamicSender(load ConfigLoader) *DynamicSender {
	return &DynamicSender{load: load, maxAttempts: 3, baseDelay: 500 * time.Millisecond}
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
	cfg, err := d.load(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("email: load config: %w", err)
	}
	sender, err := NewSender(cfg)
	if err != nil {
		return Result{}, err
	}
	provider := string(cfg.Backend)
	kind, _ := ctx.Value(kindKey).(string)

	if err := d.checkLimits(ctx, cfg.Limits); err != nil {
		d.record(ctx, Delivery{Provider: provider, To: to, Kind: kind, Status: DeliveryFailed, Error: err.Error(), ErrorKind: FailRateLimited})
		return Result{}, &SendError{Kind: FailRateLimited, Attempts: 0, Err: err}
	}

	attempts := d.maxAttempts
	if noRetry, _ := ctx.Value(noRetryKey).(bool); noRetry {
		attempts = 1
	}
	var id string
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
	if err != nil {
		k := Classify(err)
		d.record(ctx, Delivery{Provider: provider, To: to, Kind: kind, Status: DeliveryFailed, Error: err.Error(), ErrorKind: k, Attempts: n})
		return Result{}, &SendError{Kind: k, Attempts: n, Err: err}
	}
	d.record(ctx, Delivery{Provider: provider, To: to, Kind: kind, Status: DeliverySent, MessageID: id, Attempts: n})
	return Result{Provider: provider, MessageID: id, Attempts: n}, nil
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
	cfg, err := d.load(ctx)
	if err != nil {
		return false
	}
	_, err = NewSender(cfg)
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
