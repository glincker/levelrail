package plugin

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/GLINCKER/levelrail/internal/plugin/pluginpb"
)

// Call failures. Callers match with errors.Is.
var (
	ErrUnavailable = errors.New("plugin: not running")
	ErrBusy        = errors.New("plugin: too many calls in flight")
	ErrCircuitOpen = errors.New("plugin: paused after repeated failures")
	ErrTooLarge    = errors.New("plugin: payload is larger than the configured limit")
	ErrKind        = errors.New("plugin: not a notification channel")
)

// Notification is the one event a notification_channel plugin receives.
type Notification struct {
	ID         string
	Title      string
	Body       string
	Status     string
	Severity   string
	ResourceID string
	Link       string
	At         time.Time
	// SensitiveLines are log excerpts. They reach the plugin only when the
	// operator approved read:sensitive for it.
	SensitiveLines []string
}

// Delivery is the plugin's answer to a Send.
type Delivery struct {
	Delivered bool
	Detail    string
	Retryable bool
}

// Send hands one notification to an enabled notification_channel plugin. secret
// is the credential for this call only: it is never stored or logged here.
func (m *Manager) Send(ctx context.Context, name, secret string, n Notification) (Delivery, error) {
	rec, err := m.store.GetPlugin(ctx, name)
	if err != nil {
		return Delivery{}, err
	}
	if rec.Kind != KindNotificationChannel {
		return Delivery{}, ErrKind
	}
	if rec.State != StateEnabled {
		return Delivery{}, fmt.Errorf("%w: plugin is %s", ErrUnavailable, rec.State)
	}
	if !slices.Contains(rec.ApprovedAbilities, AbilityRead) {
		return Delivery{}, ErrNotApproved
	}
	if len(secret) > m.cfg.MaxSecretBytes || len(rec.ConfigJSON) > m.cfg.MaxConfigBytes {
		return Delivery{}, ErrTooLarge
	}
	in := m.inst(name)
	client, err := m.admit(in)
	if err != nil {
		return Delivery{}, err
	}
	defer func() { <-in.sem }()

	abilities := []string{AbilityRead}
	if len(n.SensitiveLines) > 0 && slices.Contains(rec.ApprovedAbilities, AbilityReadSensitive) {
		abilities = append(abilities, AbilityReadSensitive)
	} else {
		n.SensitiveLines = nil
	}
	if err := m.audit.Record(ctx, AuditEvent{Action: AuditAbilityUsed, Actor: "plugin:" + name, Plugin: name, Detail: map[string]string{
		"call": "send", "abilities": strings.Join(abilities, ","), "event": n.ID,
	}}); err != nil {
		return Delivery{}, fmt.Errorf("plugin: audit failed, call refused: %w", err)
	}

	req := &pluginpb.SendRequest{Event: m.boundedEvent(n), ConfigJson: rec.ConfigJSON, Secret: secret}
	cctx, cancel := context.WithTimeout(ctx, m.cfg.CallTimeout)
	defer cancel()
	resp, err := client.Send(cctx, req)
	if err != nil {
		m.noteCallResult(in, false)
		return Delivery{}, fmt.Errorf("plugin: send via %q failed: %w", name, redactSecret(err, secret))
	}
	m.noteCallResult(in, true)
	return Delivery{Delivered: resp.GetDelivered(), Detail: m.boundedDetail(resp.GetDetail(), secret), Retryable: resp.GetRetryable()}, nil
}

func (m *Manager) admit(in *instance) (rpcClient, error) {
	in.mu.Lock()
	client := in.client
	open := m.now().Before(in.breakerUntil)
	in.mu.Unlock()
	if client == nil {
		return nil, ErrUnavailable
	}
	if open {
		return nil, ErrCircuitOpen
	}
	select {
	case in.sem <- struct{}{}:
		return client, nil
	default:
		return nil, ErrBusy
	}
}

func (m *Manager) noteCallResult(in *instance, ok bool) {
	in.mu.Lock()
	defer in.mu.Unlock()
	if ok {
		in.breakerFails = 0
		return
	}
	in.breakerFails++
	if in.breakerFails >= m.cfg.BreakerFailures {
		in.breakerUntil = m.now().Add(m.cfg.BreakerCooldown)
		in.breakerFails = 0
	}
}

func (m *Manager) boundedEvent(n Notification) *pluginpb.Event {
	budget := m.cfg.MaxEventBytes
	title := truncateUTF8(n.Title, min(512, budget/8))
	body := n.Body
	if len(n.SensitiveLines) > 0 {
		body += "\n" + strings.Join(n.SensitiveLines, "\n")
	}
	body = truncateUTF8(body, budget-len(title)-3*512)
	at := n.At
	if at.IsZero() {
		at = m.now()
	}
	return &pluginpb.Event{
		Id: truncateUTF8(n.ID, 128), Title: title, Body: body, Status: truncateUTF8(n.Status, 32), Severity: truncateUTF8(n.Severity, 32),
		ResourceId: truncateUTF8(n.ResourceID, 256), Link: truncateUTF8(n.Link, 1024), OccurredAt: timestamppb.New(at),
	}
}

func (m *Manager) boundedDetail(detail, secret string) string {
	if secret != "" {
		detail = strings.ReplaceAll(detail, secret, "[redacted]")
	}
	return truncateUTF8(detail, m.cfg.MaxResponseBytes)
}

func redactSecret(err error, secret string) error {
	if secret == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), secret, "[redacted]"))
}

func truncateUTF8(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}
