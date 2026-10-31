package statuswebhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/statuspage"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/backoff"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// Event types a subscriber can filter on.
const (
	IncidentOpened       = "incident_opened"
	IncidentUpdated      = "incident_updated"
	IncidentResolved     = "incident_resolved"
	ComponentChanged     = "component_changed"
	MaintenanceScheduled = "maintenance_scheduled"
	MaintenanceUpdated   = "maintenance_updated"
	MaintenanceCompleted = "maintenance_completed"
	// Ping is the signed test event; it is never subject to a filter.
	Ping = "ping"
)

// EventTypes lists every type a subscriber may filter on.
var EventTypes = []string{IncidentOpened, IncidentUpdated, IncidentResolved, ComponentChanged,
	MaintenanceScheduled, MaintenanceUpdated, MaintenanceCompleted}

const (
	secretKey        = "signing_secret"
	maxErrorLen      = 300
	maxDrainBytes    = 4096
	dueBatch         = 50
	maxLabelLen      = 80
	maxURLLen        = 2000
	signatureVersion = "sha256="
)

// Sealer is the envelope-encryption surface signing secrets are stored with; *secrets.Manager satisfies it.
type Sealer interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// Store is the persistence surface; *store.DB satisfies it.
type Store interface {
	CreateStatusWebhook(ctx context.Context, w store.StatusWebhook, limit int) error
	ListStatusWebhooks(ctx context.Context) ([]store.StatusWebhook, error)
	GetStatusWebhook(ctx context.Context, id string) (store.StatusWebhook, error)
	SetStatusWebhookSecret(ctx context.Context, id, plain string, sealed bool) error
	SetStatusWebhookPaused(ctx context.Context, id string, paused bool, reason string) error
	DeleteStatusWebhook(ctx context.Context, id string) error
	AddStatusWebhookDelivery(ctx context.Context, d store.StatusWebhookDelivery) (bool, error)
	DueStatusWebhookDeliveries(ctx context.Context, now time.Time, limit int) ([]store.StatusWebhookDelivery, error)
	ListStatusWebhookDeliveries(ctx context.Context, webhookID string, limit int) ([]store.StatusWebhookDelivery, error)
	RecordStatusWebhookAttempt(ctx context.Context, d store.StatusWebhookDelivery, ok, countStreak bool, pauseAfter, keep int) (bool, error)
}

// ValidationError is a user-facing problem with a subscriber's input.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }

// ErrSealerUnavailable means a sealed secret cannot be opened because no master key is configured.
var ErrSealerUnavailable = errors.New("statuswebhook: sealed signing secret needs a master key")

// PageRef identifies the page in an event.
type PageRef struct {
	Title string `json:"title"`
	Path  string `json:"path"`
}

// ComponentRef is the public view of a component in an event.
type ComponentRef struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Change      string `json:"change"`
}

// Envelope is the JSON body posted to subscribers.
type Envelope struct {
	ID        string                   `json:"id"`
	Type      string                   `json:"type"`
	CreatedAt time.Time                `json:"created_at"`
	Page      PageRef                  `json:"page"`
	Incident  *statuspage.IncidentView `json:"incident,omitempty"`
	Component *ComponentRef            `json:"component,omitempty"`
}

// Event is what callers hand to Emit. ObjectID and Stamp make the event id deterministic.
type Event struct {
	Type      string
	ObjectID  string
	Stamp     string
	Page      PageRef
	Incident  *statuspage.IncidentView
	Component *ComponentRef
}

// Service manages subscribers and delivers events.
type Service struct {
	st     Store
	sealer func() Sealer
	cfg    Config
	client *http.Client
	logger *slog.Logger
	now    func() time.Time
}

// New builds a Service. sealer may return nil, in which case secrets are stored in the clear column.
func New(st Store, sealer func() Sealer, cfg Config, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	client := netguard.NewClientAllowing(EnvAllowPrivate)
	client.Timeout = cfg.Timeout
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Service{st: st, sealer: sealer, cfg: cfg, client: client, logger: logger, now: time.Now}
}

// Config returns the active limits.
func (s *Service) Config() Config { return s.cfg }

// Sign returns the X-Signature value for a timestamp and body.
func Sign(secret, timestamp string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(timestamp))
	m.Write([]byte("."))
	m.Write(body)
	return signatureVersion + hex.EncodeToString(m.Sum(nil))
}

// Verify reports whether signature matches, in constant time.
func Verify(secret, timestamp string, body []byte, signature string) bool {
	return hmac.Equal([]byte(Sign(secret, timestamp, body)), []byte(signature))
}

// ValidateURL applies the scheme and SSRF rules for subscriber URLs.
func (s *Service) ValidateURL(ctx context.Context, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || len(raw) > maxURLLen {
		return ValidationError("url must be an absolute http or https URL without credentials")
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !s.cfg.AllowHTTP) {
		return ValidationError("url must use https (set " + EnvAllowHTTP + "=true to allow http)")
	}
	if s.cfg.AllowPrivate {
		return nil
	}
	if err := netguard.ValidateURL(ctx, raw); err != nil {
		return ValidationError("url must point to a public host (set " + EnvAllowPrivate + "=true to allow internal targets)")
	}
	return nil
}

func normalizeEvents(in []string) ([]string, error) {
	var out []string
	for _, e := range in {
		e = strings.TrimSpace(e)
		if e == "" || slices.Contains(out, e) {
			continue
		}
		if !slices.Contains(EventTypes, e) {
			return nil, ValidationError("unknown event " + strconv.Quote(e) + ", use " + strings.Join(EventTypes, ", "))
		}
		out = append(out, e)
	}
	return out, nil
}

func randHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("statuswebhook: read random: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (s *Service) storeSecret(ctx context.Context, id, secret string) error {
	if sl := s.sealer(); sl != nil {
		if err := sl.SetValue(ctx, serviceName(id), secretKey, secret); err != nil {
			return fmt.Errorf("statuswebhook: seal secret: %w", err)
		}
		return s.st.SetStatusWebhookSecret(ctx, id, "", true)
	}
	return s.st.SetStatusWebhookSecret(ctx, id, secret, false)
}

func serviceName(id string) string { return "status-webhook/" + id }

// Create validates and stores a subscriber and returns it with its one-time signing secret.
func (s *Service) Create(ctx context.Context, rawURL, label string, events []string) (store.StatusWebhook, string, error) {
	rawURL, label = strings.TrimSpace(rawURL), strings.TrimSpace(label)
	if err := s.ValidateURL(ctx, rawURL); err != nil {
		return store.StatusWebhook{}, "", err
	}
	if len(label) > maxLabelLen {
		return store.StatusWebhook{}, "", ValidationError("label is too long")
	}
	evs, err := normalizeEvents(events)
	if err != nil {
		return store.StatusWebhook{}, "", err
	}
	idPart, err := randHex(8)
	if err != nil {
		return store.StatusWebhook{}, "", err
	}
	secPart, err := randHex(32)
	if err != nil {
		return store.StatusWebhook{}, "", err
	}
	w := store.StatusWebhook{ID: "swh_" + idPart, URL: rawURL, Label: label, Events: evs}
	if err := s.st.CreateStatusWebhook(ctx, w, s.cfg.MaxSubscribers); err != nil {
		return store.StatusWebhook{}, "", err
	}
	secret := "whsec_" + secPart
	if err := s.storeSecret(ctx, w.ID, secret); err != nil {
		_ = s.st.DeleteStatusWebhook(ctx, w.ID)
		return store.StatusWebhook{}, "", err
	}
	got, err := s.st.GetStatusWebhook(ctx, w.ID)
	return got, secret, err
}

// Rotate replaces the signing secret and returns the new one once.
func (s *Service) Rotate(ctx context.Context, id string) (string, error) {
	if _, err := s.st.GetStatusWebhook(ctx, id); err != nil {
		return "", err
	}
	part, err := randHex(32)
	if err != nil {
		return "", err
	}
	secret := "whsec_" + part
	return secret, s.storeSecret(ctx, id, secret)
}

// Delete removes a subscriber, its log and its sealed secret.
func (s *Service) Delete(ctx context.Context, id string) error {
	if sl := s.sealer(); sl != nil {
		if err := sl.DeleteAll(ctx, serviceName(id)); err != nil {
			s.logger.Warn("statuswebhook: delete sealed secret failed", slog.String("webhook", id), slog.String("error", err.Error()))
		}
	}
	return s.st.DeleteStatusWebhook(ctx, id)
}

func (s *Service) secretFor(ctx context.Context, w store.StatusWebhook) (string, error) {
	if !w.Sealed {
		return w.SecretPlain, nil
	}
	sl := s.sealer()
	if sl == nil {
		return "", ErrSealerUnavailable
	}
	return sl.Resolve(ctx, serviceName(w.ID), secretKey)
}

func matches(w store.StatusWebhook, eventType string) bool {
	return len(w.Events) == 0 || slices.Contains(w.Events, eventType)
}

// Emit queues ev for every active subscriber whose filter admits it. Repeating the
// same event is a no-op, so callers may retry freely.
func (s *Service) Emit(ctx context.Context, ev Event) error {
	subs, err := s.st.ListStatusWebhooks(ctx)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	sum := sha256.Sum256([]byte(ev.Type + "|" + ev.ObjectID + "|" + ev.Stamp))
	env := Envelope{ID: "evt_" + hex.EncodeToString(sum[:12]), Type: ev.Type, CreatedAt: now, Page: ev.Page, Incident: ev.Incident, Component: ev.Component}
	body, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("statuswebhook: encode event: %w", err)
	}
	for _, w := range subs {
		if w.Paused || !matches(w, ev.Type) {
			continue
		}
		did, err := randHex(8)
		if err != nil {
			return err
		}
		if _, err := s.st.AddStatusWebhookDelivery(ctx, store.StatusWebhookDelivery{ID: "swd_" + did, WebhookID: w.ID, EventID: env.ID,
			EventType: ev.Type, Payload: string(body), State: store.WebhookPending, NextAttemptAt: now, CreatedAt: now}); err != nil {
			return err
		}
	}
	return nil
}

// TestResult is the outcome of a test ping.
type TestResult struct {
	Delivered  bool   `json:"delivered"`
	StatusCode int    `json:"status_code"`
	Error      string `json:"error,omitempty"`
}

// Test sends one signed ping now and logs it. It never counts toward auto-pause.
func (s *Service) Test(ctx context.Context, id string, page PageRef) (TestResult, error) {
	w, err := s.st.GetStatusWebhook(ctx, id)
	if err != nil {
		return TestResult{}, err
	}
	now := s.now().UTC()
	rid, err := randHex(12)
	if err != nil {
		return TestResult{}, err
	}
	body, err := json.Marshal(Envelope{ID: "evt_" + rid, Type: Ping, CreatedAt: now, Page: page})
	if err != nil {
		return TestResult{}, fmt.Errorf("statuswebhook: encode ping: %w", err)
	}
	code, sendErr := s.send(ctx, w, "evt_"+rid, Ping, body)
	d := store.StatusWebhookDelivery{ID: "swd_" + rid[:16], WebhookID: w.ID, EventID: "evt_" + rid, EventType: Ping, Payload: string(body),
		State: store.WebhookDelivered, Attempts: 1, NextAttemptAt: now, LastStatus: code, CreatedAt: now, DeliveredAt: &now}
	res := TestResult{Delivered: sendErr == nil, StatusCode: code}
	if sendErr != nil {
		d.State, d.DeliveredAt, d.LastError = store.WebhookFailed, nil, trimErr(sendErr)
		res.Error = d.LastError
	}
	if _, err := s.st.AddStatusWebhookDelivery(ctx, d); err != nil {
		return res, err
	}
	return res, nil
}

func trimErr(err error) string {
	msg := err.Error()
	if len(msg) > maxErrorLen {
		msg = msg[:maxErrorLen]
	}
	return msg
}

// send posts one signed body and returns the HTTP status (0 when no response arrived).
func (s *Service) send(ctx context.Context, w store.StatusWebhook, eventID, eventType string, body []byte) (int, error) {
	secret, err := s.secretFor(ctx, w)
	if err != nil {
		return 0, err
	}
	if err := s.ValidateURL(ctx, w.URL); err != nil {
		return 0, err
	}
	ts := strconv.FormatInt(s.now().Unix(), 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("statuswebhook: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Signature", Sign(secret, ts, body))
	req.Header.Set("X-Event-Id", eventID)
	req.Header.Set("X-Event-Type", eventType)
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	// The body is read only to let the connection be reused; it is never stored.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainBytes))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.StatusCode, fmt.Errorf("receiver answered HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// ProcessDue attempts every delivery whose time has come and returns how many it tried.
func (s *Service) ProcessDue(ctx context.Context) (int, error) {
	due, err := s.st.DueStatusWebhookDeliveries(ctx, s.now().UTC(), dueBatch)
	if err != nil {
		return 0, err
	}
	for _, d := range due {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		s.attempt(ctx, d)
	}
	return len(due), nil
}

func (s *Service) attempt(ctx context.Context, d store.StatusWebhookDelivery) {
	now := s.now().UTC()
	w, err := s.st.GetStatusWebhook(ctx, d.WebhookID)
	if err != nil || w.Paused {
		d.State, d.LastError = store.WebhookFailed, "subscriber paused or removed"
		if _, rerr := s.st.RecordStatusWebhookAttempt(ctx, d, true, false, 0, s.cfg.LogKeep); rerr != nil && !errors.Is(err, store.ErrStatusWebhookNotFound) {
			s.logger.Warn("statuswebhook: record skipped delivery failed", slog.String("error", rerr.Error()))
		}
		return
	}
	code, sendErr := s.send(ctx, w, d.EventID, d.EventType, []byte(d.Payload))
	d.Attempts++
	d.LastStatus = code
	switch {
	case sendErr == nil:
		d.State, d.LastError, d.DeliveredAt = store.WebhookDelivered, "", &now
	case d.Attempts >= s.cfg.MaxAttempts:
		d.State, d.LastError = store.WebhookFailed, trimErr(sendErr)
	default:
		d.State, d.LastError = store.WebhookPending, trimErr(sendErr)
		d.NextAttemptAt = now.Add(backoff.Exponential(s.cfg.BackoffBase, s.cfg.BackoffMax, d.Attempts))
	}
	// Only a delivery that has used all its attempts counts toward the streak, so one
	// slow receiver is not paused by a single event's retries.
	count := sendErr == nil || d.State == store.WebhookFailed
	if !count {
		if _, err := s.st.RecordStatusWebhookAttempt(ctx, d, true, false, 0, 0); err != nil {
			s.logger.Warn("statuswebhook: record attempt failed", slog.String("error", err.Error()))
		}
		return
	}
	paused, err := s.st.RecordStatusWebhookAttempt(ctx, d, sendErr == nil, true, s.cfg.PauseAfter, s.cfg.LogKeep)
	if err != nil {
		s.logger.Warn("statuswebhook: record attempt failed", slog.String("error", err.Error()))
		return
	}
	if paused {
		s.logger.Warn("statuswebhook: subscriber paused after repeated failures", slog.String("webhook", w.ID))
	}
}

// Run delivers queued events until ctx ends. It is one goroutine and returns when ctx does.
func (s *Service) Run(ctx context.Context) error {
	t := time.NewTicker(s.cfg.Poll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if _, err := s.ProcessDue(ctx); err != nil && ctx.Err() == nil {
				s.logger.Warn("statuswebhook: delivery pass failed", slog.String("error", err.Error()))
			}
		}
	}
}

// Deliveries returns a subscriber's log, newest first.
func (s *Service) Deliveries(ctx context.Context, id string) ([]store.StatusWebhookDelivery, error) {
	if _, err := s.st.GetStatusWebhook(ctx, id); err != nil {
		return nil, err
	}
	return s.st.ListStatusWebhookDeliveries(ctx, id, s.cfg.LogKeep)
}
