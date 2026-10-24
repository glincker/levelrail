package eventhook

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dataexport"
	"github.com/GLINCKER/levelrail/internal/store"
)

// knownEventTypes seeds the UI picker; subscriptions also accept any
// "family.*" prefix or exact type the platform emits.
var knownEventTypes = []string{
	"apps.create", "apps.update", "apps.delete", "apps.deploy", "apps.rollback",
	"databases.create", "databases.delete",
	"users.create", "users.update", "users.delete",
	"tokens.create", "tokens.delete",
	"settings.update",
	"cleanup.run", "cleanup.failed",
}

// TypeForRequest names the event for one audited mutation: the audit action
// when the handler set one, else "<resource>.<verb>" from the request path.
func TypeForRequest(method, path, action string) string {
	if action != "" {
		return action
	}
	segs := strings.Split(strings.Trim(strings.TrimPrefix(path, "/api/v1"), "/"), "/")
	if len(segs) == 0 || segs[0] == "" {
		return "api.request"
	}
	verb := map[string]string{
		http.MethodPost: "create", http.MethodPut: "update",
		http.MethodPatch: "update", http.MethodDelete: "delete",
	}[method]
	if verb == "" {
		return ""
	}
	if len(segs) > 2 && method == http.MethodPost {
		verb = segs[len(segs)-1]
	}
	return segs[0] + "." + verb
}

// Input is the writable part of a webhook.
type Input struct {
	Name    string
	URL     string
	Enabled bool
	Events  []string
}

// Patch holds the optional changes of an update.
type Patch struct {
	Name    *string
	URL     *string
	Enabled *bool
	Events  *[]string
}

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

func validate(ctx context.Context, name, url string, events []string) error {
	if name == "" || len(name) > maxNameLen {
		return invalid("name is required and must be at most %d characters", maxNameLen)
	}
	if err := dataexport.ValidateHTTPEndpoint(ctx, url); err != nil {
		return invalid("%v", err)
	}
	for _, e := range events {
		if strings.TrimSpace(e) == "" || strings.ContainsAny(e, " \t") {
			return invalid("event type %q is not valid", e)
		}
	}
	return nil
}

func newSecret() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("eventhook: generate signing secret: %w", err)
	}
	return "whsec_" + hex.EncodeToString(buf), nil
}

// Create validates and stores a webhook and returns it with its signing
// secret, which is shown once and never readable afterwards.
func (s *Service) Create(ctx context.Context, in Input) (store.EventWebhook, string, error) {
	in.Name = strings.TrimSpace(in.Name)
	if err := validate(ctx, in.Name, in.URL, in.Events); err != nil {
		return store.EventWebhook{}, "", err
	}
	if s.d.Secrets == nil {
		return store.EventWebhook{}, "", ErrSecretsUnavailable
	}
	id, err := store.NewEventWebhookID()
	if err != nil {
		return store.EventWebhook{}, "", err
	}
	secret, err := newSecret()
	if err != nil {
		return store.EventWebhook{}, "", err
	}
	if err := s.d.Secrets.SetValue(ctx, store.EventWebhookSecretsKey(id), secretEnvKey, secret); err != nil {
		return store.EventWebhook{}, "", fmt.Errorf("eventhook: store signing secret: %w", err)
	}
	now := store.FormatAuditTime(s.d.Now())
	w := store.EventWebhook{ID: id, Name: in.Name, URL: in.URL, Enabled: in.Enabled, Events: in.Events, CreatedAt: now, UpdatedAt: now}
	if err := s.d.Store.SaveEventWebhook(ctx, w); err != nil {
		_ = s.d.Secrets.DeleteAll(ctx, store.EventWebhookSecretsKey(id))
		return store.EventWebhook{}, "", err
	}
	return w, secret, nil
}

// Update applies a patch.
func (s *Service) Update(ctx context.Context, id string, p Patch) (store.EventWebhook, error) {
	w, err := s.d.Store.GetEventWebhook(ctx, id)
	if err != nil {
		return store.EventWebhook{}, err
	}
	if p.Name != nil {
		w.Name = strings.TrimSpace(*p.Name)
	}
	if p.URL != nil {
		w.URL = *p.URL
	}
	if p.Enabled != nil {
		w.Enabled = *p.Enabled
	}
	if p.Events != nil {
		w.Events = *p.Events
	}
	if err := validate(ctx, w.Name, w.URL, w.Events); err != nil {
		return store.EventWebhook{}, err
	}
	w.UpdatedAt = store.FormatAuditTime(s.d.Now())
	if err := s.d.Store.UpdateEventWebhook(ctx, w); err != nil {
		return store.EventWebhook{}, err
	}
	return w, nil
}

// RotateSecret issues a new signing secret and returns it once.
func (s *Service) RotateSecret(ctx context.Context, id string) (string, error) {
	if _, err := s.d.Store.GetEventWebhook(ctx, id); err != nil {
		return "", err
	}
	if s.d.Secrets == nil {
		return "", ErrSecretsUnavailable
	}
	secret, err := newSecret()
	if err != nil {
		return "", err
	}
	if err := s.d.Secrets.SetValue(ctx, store.EventWebhookSecretsKey(id), secretEnvKey, secret); err != nil {
		return "", fmt.Errorf("eventhook: store signing secret: %w", err)
	}
	return secret, nil
}

// Delete removes a webhook, its delivery log and its secret.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.d.Store.DeleteEventWebhook(ctx, id); err != nil {
		return err
	}
	if s.d.Secrets != nil {
		if err := s.d.Secrets.DeleteAll(ctx, store.EventWebhookSecretsKey(id)); err != nil {
			return fmt.Errorf("eventhook: delete signing secret: %w", err)
		}
	}
	return nil
}

// Get returns one webhook.
func (s *Service) Get(ctx context.Context, id string) (store.EventWebhook, error) {
	return s.d.Store.GetEventWebhook(ctx, id)
}

// List returns every webhook.
func (s *Service) List(ctx context.Context) ([]store.EventWebhook, error) {
	return s.d.Store.ListEventWebhooks(ctx)
}

// Counts summarises delivery state for the list view.
type Counts struct {
	Pending, Delivered, Dead int
	LastStatus               int
	LastAt                   time.Time
}

// Summary tallies a webhook's recent deliveries.
func (s *Service) Summary(ctx context.Context, id string) (Counts, error) {
	ds, err := s.d.Store.ListEventDeliveries(ctx, id, 100)
	if err != nil {
		return Counts{}, err
	}
	var c Counts
	for i, d := range ds {
		switch d.State {
		case store.EventDeliveryPending:
			c.Pending++
		case store.EventDeliveryDelivered:
			c.Delivered++
		case store.EventDeliveryDead:
			c.Dead++
		}
		if i == 0 {
			c.LastStatus = d.ResponseStatus
			c.LastAt, _ = time.Parse(time.RFC3339Nano, d.UpdatedAt)
		}
	}
	return c, nil
}
