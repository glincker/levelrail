// Package webpush sends browser push notifications (RFC 8030/8291) to
// every subscription this control plane's dashboard has registered,
// backing the "webpush" notification-channel kind in internal/alerting:
// an operator gets a deploy/alert notification even with no dashboard
// tab focused or open, something none of the other 17 channel kinds (all
// webhooks or email) can do on their own.
package webpush

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	webpushgo "github.com/SherClockHolmes/webpush-go"

	"github.com/GLINCKER/levelrail/internal/netguard"
	"github.com/GLINCKER/levelrail/internal/store"
)

// secretsServiceName and the two env keys below are the internal/secrets
// slot the VAPID keypair lives in (service "webpush"), the same
// envelope-encryption path every other channel credential in this
// package family would use if it needed one; this is the only
// notification-channel kind that does, since it's the only one whose
// credential this control plane generates for itself rather than an
// operator pasting one in.
const (
	secretsServiceName = "webpush"
	vapidPrivateKeyEnv = "vapid_private_key"
	vapidPublicKeyEnv  = "vapid_public_key"
)

// KeyStore is the narrow internal/secrets surface EnsureVAPIDKeys needs.
// *secrets.Manager satisfies it structurally, the same "narrow,
// consumer-defined interface" shape VaultSecrets (internal/api) already
// establishes.
type KeyStore interface {
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// EnsureVAPIDKeys returns this control plane's VAPID keypair, generating
// and persisting one through keys the first time it's called. Call once
// at startup, before anything can register a subscription against the
// public half: regenerating the pair later would silently orphan every
// subscription already registered against the old one (the browser's
// applicationServerKey would no longer match what this control plane
// signs with).
func EnsureVAPIDKeys(ctx context.Context, keys KeyStore) (publicKey, privateKey string, err error) {
	exists, err := keys.Exists(ctx, secretsServiceName, vapidPrivateKeyEnv)
	if err != nil {
		return "", "", fmt.Errorf("webpush: check vapid keys: %w", err)
	}
	if exists {
		publicKey, err = keys.Resolve(ctx, secretsServiceName, vapidPublicKeyEnv)
		if err != nil {
			return "", "", fmt.Errorf("webpush: resolve vapid public key: %w", err)
		}
		privateKey, err = keys.Resolve(ctx, secretsServiceName, vapidPrivateKeyEnv)
		if err != nil {
			return "", "", fmt.Errorf("webpush: resolve vapid private key: %w", err)
		}
		return publicKey, privateKey, nil
	}

	privateKey, publicKey, err = webpushgo.GenerateVAPIDKeys()
	if err != nil {
		return "", "", fmt.Errorf("webpush: generate vapid keys: %w", err)
	}
	if err := keys.SetValue(ctx, secretsServiceName, vapidPrivateKeyEnv, privateKey); err != nil {
		return "", "", fmt.Errorf("webpush: save vapid private key: %w", err)
	}
	if err := keys.SetValue(ctx, secretsServiceName, vapidPublicKeyEnv, publicKey); err != nil {
		return "", "", fmt.Errorf("webpush: save vapid public key: %w", err)
	}
	return publicKey, privateKey, nil
}

// Subscriptions is the store surface Sender needs: every registered
// subscription, and a way to prune one a push service has reported
// permanently gone. *store.DB satisfies this structurally.
type Subscriptions interface {
	ListPushSubscriptions(ctx context.Context) ([]store.PushSubscription, error)
	DeletePushSubscriptionByID(ctx context.Context, id string) error
}

// Sender sends one notification to every registered browser
// subscription. It satisfies alerting.PushSender structurally (that
// package takes a narrow Send(ctx, title, body) interface rather than
// importing this one, the same "consumer defines the interface it needs"
// shape email.Sender's own consumers already follow).
type Sender struct {
	subs       Subscriptions
	publicKey  string
	privateKey string
	subscriber string
	client     webpushgo.HTTPClient
	logger     *slog.Logger
}

// NewSender builds a Sender. subscriber is the VAPID JWT "sub" claim a
// push service may contact if this control plane's server is sending too
// aggressively; an https:// URL or an email address, per RFC 8292.
func NewSender(subs Subscriptions, publicKey, privateKey, subscriber string, client *http.Client, logger *slog.Logger) *Sender {
	if client == nil {
		client = netguard.NewClient()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Sender{subs: subs, publicKey: publicKey, privateKey: privateKey, subscriber: subscriber, client: client, logger: logger}
}

// pushPayload is the JSON body this control plane's own push-sw.js
// (web/public/push-sw.js) expects in the push event's data: a title and
// body are everything a Notification needs to display.
type pushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// pushTTLSeconds bounds how long a push service holds an undelivered
// message for an offline browser before discarding it. A deploy/alert
// notification is only ever useful fresh, so there is no reason to hold
// one past a few minutes.
const pushTTLSeconds = 300

// Send pushes one notification to every registered subscription.
// A per-subscription failure never aborts the loop: one dead or revoked
// browser must not silence every other one. Returns an error only when
// no registered subscription exists at all, or every send attempt
// failed, mirroring email.Sender's own "nothing configured" failure
// shape that NewDeployDispatcher's doc comment already describes for
// every other channel kind.
func (s *Sender) Send(ctx context.Context, title, body string) error {
	if s.privateKey == "" {
		return errors.New("webpush: vapid keys are not configured on this control plane")
	}
	subs, err := s.subs.ListPushSubscriptions(ctx)
	if err != nil {
		return fmt.Errorf("webpush: list subscriptions: %w", err)
	}
	if len(subs) == 0 {
		return errors.New("webpush: no browser has registered for push notifications yet")
	}

	payload, err := json.Marshal(pushPayload{Title: title, Body: body})
	if err != nil {
		return fmt.Errorf("webpush: encode payload: %w", err)
	}

	var lastErr error
	sent := 0
	for _, sub := range subs {
		if err := s.sendOne(ctx, sub, payload); err != nil {
			lastErr = err
			s.logger.Error("webpush: send failed",
				slog.String("subscription_id", sub.ID), slog.String("error", err.Error()))
			continue
		}
		sent++
	}
	if sent == 0 {
		return fmt.Errorf("webpush: every subscription failed, last error: %w", lastErr)
	}
	return nil
}

func (s *Sender) sendOne(ctx context.Context, sub store.PushSubscription, payload []byte) error {
	resp, err := webpushgo.SendNotificationWithContext(ctx, payload, &webpushgo.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpushgo.Keys{Auth: sub.Auth, P256dh: sub.P256dh},
	}, &webpushgo.Options{
		HTTPClient:      s.client,
		Subscriber:      s.subscriber,
		TTL:             pushTTLSeconds,
		VAPIDPublicKey:  s.publicKey,
		VAPIDPrivateKey: s.privateKey,
	})
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// A push service returns 404/410 once a subscription is permanently
	// gone (uninstalled browser, revoked permission): resending to it
	// forever would just accumulate dead rows.
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		if delErr := s.subs.DeletePushSubscriptionByID(ctx, sub.ID); delErr != nil {
			s.logger.Error("webpush: prune expired subscription failed",
				slog.String("subscription_id", sub.ID), slog.String("error", delErr.Error()))
		}
		return fmt.Errorf("subscription expired (status %d), pruned", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("receiver returned status %d", resp.StatusCode)
	}
	return nil
}
