// Package eventhook delivers platform events to operator-configured HTTPS
// endpoints: signed, queued, retried with backoff, dead-lettered after the
// last attempt, and recorded in a delivery log that supports replay.
package eventhook

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/dataexport"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/backoff"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

const secretEnvKey = "signing"

const maxNameLen = 80

// ErrInvalid wraps every validation failure so callers can answer 400.
var ErrInvalid = errors.New("invalid event webhook")

// ErrSecretsUnavailable is returned when no master key is configured.
var ErrSecretsUnavailable = errors.New("eventhook: secret storage is not configured")

// Store is the persistence surface; *store.DB satisfies it.
type Store interface {
	SaveEventWebhook(ctx context.Context, w store.EventWebhook) error
	UpdateEventWebhook(ctx context.Context, w store.EventWebhook) error
	GetEventWebhook(ctx context.Context, id string) (store.EventWebhook, error)
	ListEventWebhooks(ctx context.Context) ([]store.EventWebhook, error)
	DeleteEventWebhook(ctx context.Context, id string) error
	SaveEventDelivery(ctx context.Context, d store.EventWebhookDelivery) error
	UpdateEventDelivery(ctx context.Context, d store.EventWebhookDelivery) error
	GetEventDelivery(ctx context.Context, id string) (store.EventWebhookDelivery, error)
	ListDueEventDeliveries(ctx context.Context, now string, limit int) ([]store.EventWebhookDelivery, error)
	ListEventDeliveries(ctx context.Context, webhookID string, limit int) ([]store.EventWebhookDelivery, error)
	DeleteEventDeliveriesOlderThan(ctx context.Context, cutoff string) (int64, error)
}

// Secrets stores signing secrets in envelope encryption.
type Secrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// Deps wires a Service.
type Deps struct {
	Store   Store
	Secrets Secrets
	Config  Config
	// RetentionDays, when set, supplies the live delivery retention in days (0 keeps everything) instead of Config.Retention.
	RetentionDays func(ctx context.Context) int
	Brand         string
	Logger        *slog.Logger
	HTTPClient    *http.Client
	Now           func() time.Time
}

// Event is one thing that happened, as handed to Emit.
type Event struct {
	Type string
	Data map[string]any
}

// Service owns webhook CRUD and the delivery workers.
type Service struct {
	d       Deps
	hdrPfx  string
	client  *http.Client
	intake  chan Event
	dropped atomic.Int64
}

// NewService builds a Service. Call Run to start delivery.
func NewService(d Deps) *Service {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Config.Tick == 0 {
		d.Config = DefaultConfig()
	}
	client := d.HTTPClient
	if client == nil {
		client = netguard.NewClientAllowing(dataexport.AllowPrivateEnv, netguard.AllowPrivateEnv)
		client.Timeout = d.Config.SendTimeout
	}
	brand := d.Brand
	if brand == "" {
		brand = "platform"
	}
	return &Service{
		d: d, client: client,
		hdrPfx: "X-" + strings.ToUpper(brand[:1]) + strings.ToLower(brand[1:]),
		intake: make(chan Event, d.Config.QueueSize),
	}
}

// Emit queues an event without blocking the caller. When the intake buffer
// is full the event is dropped and counted, never waited on.
func (s *Service) Emit(e Event) {
	select {
	case s.intake <- e:
	default:
		s.dropped.Add(1)
	}
}

// Dropped reports how many events were discarded because the buffer was full.
func (s *Service) Dropped() int64 { return s.dropped.Load() }

// Run fans events out to delivery rows and sends due deliveries until ctx ends.
func (s *Service) Run(ctx context.Context) {
	tick := time.NewTicker(s.d.Config.Tick)
	defer tick.Stop()
	prune := time.NewTicker(time.Hour)
	defer prune.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-s.intake:
			s.fanOut(ctx, e)
		case <-tick.C:
			s.deliverDue(ctx)
		case <-prune.C:
			retention := s.d.Config.Retention
			if s.d.RetentionDays != nil {
				days := s.d.RetentionDays(ctx)
				if days <= 0 {
					continue
				}
				retention = time.Duration(days) * 24 * time.Hour
			}
			cutoff := store.FormatAuditTime(s.d.Now().Add(-retention))
			if _, err := s.d.Store.DeleteEventDeliveriesOlderThan(ctx, cutoff); err != nil {
				s.d.Logger.Warn("eventhook: prune deliveries", slog.String("error", err.Error()))
			}
		}
	}
}

// Matches reports whether a subscription list selects an event type. An
// empty list selects everything; "family.*" selects a name prefix.
func Matches(subscribed []string, eventType string) bool {
	if len(subscribed) == 0 {
		return true
	}
	for _, p := range subscribed {
		switch {
		case p == "*" || p == eventType:
			return true
		case strings.HasSuffix(p, ".*") && strings.HasPrefix(eventType, strings.TrimSuffix(p, "*")):
			return true
		}
	}
	return false
}

func (s *Service) fanOut(ctx context.Context, e Event) {
	hooks, err := s.d.Store.ListEventWebhooks(ctx)
	if err != nil {
		s.d.Logger.Warn("eventhook: list webhooks", slog.String("error", err.Error()))
		return
	}
	var eventID string
	var payload []byte
	for _, w := range hooks {
		if !w.Enabled || !Matches(w.Events, e.Type) {
			continue
		}
		if payload == nil {
			if eventID, payload, err = s.envelope(e); err != nil {
				s.d.Logger.Warn("eventhook: encode event", slog.String("type", e.Type), slog.String("error", err.Error()))
				return
			}
		}
		if err := s.enqueue(ctx, w.ID, eventID, e.Type, payload); err != nil {
			s.d.Logger.Warn("eventhook: queue delivery", slog.String("webhook", w.ID), slog.String("error", err.Error()))
		}
	}
}

func (s *Service) envelope(e Event) (string, []byte, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, fmt.Errorf("eventhook: generate event id: %w", err)
	}
	id := "evt_" + hex.EncodeToString(buf)
	body, err := json.Marshal(map[string]any{
		"id": id, "type": e.Type, "created_at": s.d.Now().UTC().Format(time.RFC3339), "data": e.Data,
	})
	if err != nil {
		return "", nil, fmt.Errorf("eventhook: encode envelope: %w", err)
	}
	return id, body, nil
}

func (s *Service) enqueue(ctx context.Context, webhookID, eventID, eventType string, payload []byte) error {
	id, err := store.NewEventDeliveryID()
	if err != nil {
		return err
	}
	now := store.FormatAuditTime(s.d.Now())
	return s.d.Store.SaveEventDelivery(ctx, store.EventWebhookDelivery{
		ID: id, WebhookID: webhookID, EventID: eventID, EventType: eventType, Payload: string(payload),
		State: store.EventDeliveryPending, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	})
}

// Outcome is the decision after one send attempt.
type Outcome struct {
	State     string
	NextAfter time.Duration
}

// Decide maps an attempt result to the delivery's next state. attempts is
// the count including the attempt just made; permanent failures (a 4xx that
// a retry cannot fix) go straight to the dead letter state.
func Decide(cfg Config, attempts, status int, sendErr error) Outcome {
	if sendErr == nil && status >= 200 && status < 300 {
		return Outcome{State: store.EventDeliveryDelivered}
	}
	permanent := sendErr == nil && status >= 400 && status < 500 && status != http.StatusRequestTimeout && status != http.StatusTooManyRequests
	if permanent || attempts >= cfg.MaxAttempts {
		return Outcome{State: store.EventDeliveryDead}
	}
	return Outcome{State: store.EventDeliveryPending, NextAfter: Backoff(cfg, attempts)}
}

// Backoff is the wait before attempt number attempts+1: exponential from
// BackoffBase, capped at BackoffMax.
func Backoff(cfg Config, attempts int) time.Duration {
	return backoff.Exponential(cfg.BackoffBase, cfg.BackoffMax, attempts)
}

func (s *Service) deliverDue(ctx context.Context) {
	due, err := s.d.Store.ListDueEventDeliveries(ctx, store.FormatAuditTime(s.d.Now()), s.d.Config.BatchSize)
	if err != nil {
		s.d.Logger.Warn("eventhook: list due deliveries", slog.String("error", err.Error()))
		return
	}
	for _, dl := range due {
		if ctx.Err() != nil {
			return
		}
		s.attempt(ctx, dl)
	}
}

func (s *Service) attempt(ctx context.Context, dl store.EventWebhookDelivery) {
	w, err := s.d.Store.GetEventWebhook(ctx, dl.WebhookID)
	status, sendErr := 0, err
	if err == nil {
		status, sendErr = s.send(ctx, w, dl.EventID, dl.EventType, []byte(dl.Payload))
	}
	dl.Attempts++
	dl.ResponseStatus = status
	dl.LastError = ""
	if sendErr != nil {
		dl.LastError = truncate(sendErr.Error(), 300)
	} else if status < 200 || status >= 300 {
		dl.LastError = fmt.Sprintf("endpoint answered HTTP %d", status)
	}
	out := Decide(s.d.Config, dl.Attempts, status, sendErr)
	dl.State = out.State
	now := s.d.Now()
	dl.UpdatedAt = store.FormatAuditTime(now)
	dl.NextAttemptAt = store.FormatAuditTime(now.Add(out.NextAfter))
	if err := s.d.Store.UpdateEventDelivery(ctx, dl); err != nil {
		// The receiver may already have the event; the row stays pending and
		// is re-sent with the same event id so the receiver can dedupe.
		s.d.Logger.Warn("eventhook: record delivery result", slog.String("delivery", dl.ID), slog.String("error", err.Error()))
	}
}

func (s *Service) signingSecret(ctx context.Context, id string) (string, error) {
	if s.d.Secrets == nil {
		return "", ErrSecretsUnavailable
	}
	v, err := s.d.Secrets.Resolve(ctx, store.EventWebhookSecretsKey(id), secretEnvKey)
	if err != nil {
		return "", fmt.Errorf("eventhook: resolve signing secret: %w", err)
	}
	return v, nil
}

func (s *Service) send(ctx context.Context, w store.EventWebhook, eventID, eventType string, body []byte) (int, error) {
	secret, err := s.signingSecret(ctx, w.ID)
	if err != nil {
		return 0, err
	}
	sendCtx, cancel := context.WithTimeout(ctx, s.d.Config.SendTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(sendCtx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("eventhook: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(s.hdrPfx+"-Event", eventType)
	req.Header.Set(s.hdrPfx+"-Event-Id", eventID)
	req.Header.Set(s.hdrPfx+"-Signature", dataexport.Sign(secret, s.d.Now(), body))
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("eventhook: send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Test sends one probe event synchronously and reports the result.
func (s *Service) Test(ctx context.Context, id string) (int, error) {
	w, err := s.d.Store.GetEventWebhook(ctx, id)
	if err != nil {
		return 0, err
	}
	eventID, payload, err := s.envelope(Event{Type: "webhook.test", Data: map[string]any{"webhook": w.Name}})
	if err != nil {
		return 0, err
	}
	status, err := s.send(ctx, w, eventID, "webhook.test", payload)
	if err != nil {
		return status, err
	}
	if status < 200 || status >= 300 {
		return status, fmt.Errorf("endpoint answered HTTP %d", status)
	}
	return status, nil
}

// Replay puts a delivery back in the queue with a fresh attempt budget.
func (s *Service) Replay(ctx context.Context, deliveryID string) (store.EventWebhookDelivery, error) {
	dl, err := s.d.Store.GetEventDelivery(ctx, deliveryID)
	if err != nil {
		return store.EventWebhookDelivery{}, err
	}
	now := store.FormatAuditTime(s.d.Now())
	dl.State, dl.Attempts, dl.LastError, dl.ResponseStatus = store.EventDeliveryPending, 0, "", 0
	dl.NextAttemptAt, dl.UpdatedAt = now, now
	if err := s.d.Store.UpdateEventDelivery(ctx, dl); err != nil {
		return store.EventWebhookDelivery{}, err
	}
	return dl, nil
}

// Deliveries lists a webhook's newest deliveries.
func (s *Service) Deliveries(ctx context.Context, webhookID string, limit int) ([]store.EventWebhookDelivery, error) {
	if _, err := s.d.Store.GetEventWebhook(ctx, webhookID); err != nil {
		return nil, err
	}
	return s.d.Store.ListEventDeliveries(ctx, webhookID, limit)
}

// EventTypes lists the event types the platform emits today, sorted.
func EventTypes() []string {
	out := append([]string(nil), knownEventTypes...)
	sort.Strings(out)
	return out
}
