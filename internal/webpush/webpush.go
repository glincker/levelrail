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
	"github.com/GLINCKER/levelrail/internal/secrets"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	webpushgo "github.com/SherClockHolmes/webpush-go"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// secretsServiceName and the two env keys below are the internal/secrets
// slot the VAPID keypair lives in (service "webpush"), the same
// envelope-encryption path every other channel credential in this
// package family would use if it needed one; this is the only
// notification-channel kind that does, since it's the only one whose
// credential this control plane generates for itself rather than an
// operator pasting one in.
const (
	secretsServiceName = secrets.ControlPlanePrefix + "webpush"
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
	policy     RecipientPolicy
	validate   func(endpoint string) error
	timeout    time.Duration
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
	timeout := SendTimeout(os.Getenv)
	// A copy, so the caller's shared client keeps its redirect behaviour.
	own := *client
	own.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	own.Timeout = timeout
	allowed := AllowedHosts(os.Getenv)
	return &Sender{subs: subs, timeout: timeout, validate: func(e string) error { return ValidateEndpoint(e, allowed) }, publicKey: publicKey, privateKey: privateKey, subscriber: subscriber, client: &own, logger: logger}
}

// RecipientPolicy decides whether a user may receive a push for an audience.
type RecipientPolicy interface {
	AllowPush(ctx context.Context, userID string, audience alerting.PushAudience) bool
}

// WithEndpointValidator replaces the endpoint check run before every send.
func (s *Sender) WithEndpointValidator(fn func(endpoint string) error) *Sender {
	s.validate = fn
	return s
}

// pruneTimeout gives removing a dead subscription its own deadline, apart from the send one.
const pruneTimeout = 5 * time.Second

// WithPolicy sets the recipient policy. Without one nobody receives a push.
func (s *Sender) WithPolicy(p RecipientPolicy) *Sender {
	s.policy = p
	return s
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

// Payload bounds: a push message must fit one encrypted record.
const (
	maxTitleBytes   = 256
	maxPayloadBytes = 3000
)

// Send pushes one notification to the subscriptions of users the audience
// allows. A per-subscription failure never aborts the others, and the whole
// call stops at the send timeout. Returns an error only when no eligible
// subscription exists, or every send attempt failed.
func (s *Sender) Send(ctx context.Context, title, body string, audience alerting.PushAudience) error {
	if s.privateKey == "" {
		return errors.New("webpush: vapid keys are not configured on this control plane")
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	subs, err := s.subs.ListPushSubscriptions(ctx)
	if err != nil {
		return fmt.Errorf("webpush: list subscriptions: %w", err)
	}
	subs = s.eligible(ctx, subs, audience)
	if len(subs) == 0 {
		return errors.New("webpush: no registered browser may receive this notification")
	}

	payload, err := encodePayload(title, body)
	if err != nil {
		return fmt.Errorf("webpush: encode payload: %w", err)
	}

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		lastErr error
		sent    int
		slots   = make(chan struct{}, sendConcurrency)
	)
	for _, sub := range subs {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			err := s.sendOne(ctx, sub, payload)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				lastErr = err
				s.logger.Error("webpush: send failed",
					slog.String("subscription_id", sub.ID), slog.String("error", err.Error()))
				return
			}
			sent++
		}()
	}
	wg.Wait()
	if sent == 0 {
		return fmt.Errorf("webpush: every subscription failed, last error: %w", lastErr)
	}
	return nil
}

// eligible keeps the subscriptions whose owner the policy still allows, asking once per user.
func (s *Sender) eligible(ctx context.Context, subs []store.PushSubscription, audience alerting.PushAudience) []store.PushSubscription {
	if s.policy == nil {
		return nil
	}
	verdict := map[string]bool{}
	out := subs[:0:0]
	for _, sub := range subs {
		ok, seen := verdict[sub.UserID]
		if !seen {
			ok = s.policy.AllowPush(ctx, sub.UserID, audience)
			verdict[sub.UserID] = ok
		}
		if ok {
			out = append(out, sub)
		}
	}
	return out
}

// encodePayload cuts the title and body so the JSON stays under the record limit.
func encodePayload(title, body string) ([]byte, error) {
	title = cutBytes(title, maxTitleBytes)
	for {
		out, err := json.Marshal(pushPayload{Title: title, Body: body})
		if err != nil || len(out) <= maxPayloadBytes || body == "" {
			return out, err
		}
		// Shrink in proportion to the overshoot, so escaped bytes are accounted for.
		body = cutBytes(body, min(len(body)*maxPayloadBytes/len(out), len(body)-1))
	}
}

// cutBytes shortens s to at most n bytes without splitting a rune.
func cutBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// scrubError drops the request URL a transport error carries, so the endpoint never reaches logs or delivery records.
func scrubError(err error, endpoint string) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	return errors.New(strings.ReplaceAll(err.Error(), endpoint, "[endpoint]"))
}

func (s *Sender) sendOne(ctx context.Context, sub store.PushSubscription, payload []byte) error {
	// Rows saved before the host list existed are re-checked, and dropped when they fail.
	if s.validate != nil {
		if err := s.validate(sub.Endpoint); err != nil {
			s.prune(ctx, sub.ID)
			return errors.New("subscription endpoint not allowed, pruned")
		}
	}
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
		return fmt.Errorf("send: %w", scrubError(err, sub.Endpoint))
	}
	defer func() { _ = resp.Body.Close() }()

	// A push service returns 404/410 once a subscription is permanently
	// gone (uninstalled browser, revoked permission): resending to it
	// forever would just accumulate dead rows.
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		s.prune(ctx, sub.ID)
		return fmt.Errorf("subscription expired (status %d), pruned", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("receiver returned status %d", resp.StatusCode)
	}
	return nil
}

func (s *Sender) prune(ctx context.Context, id string) {
	pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pruneTimeout)
	defer cancel()
	if err := s.subs.DeletePushSubscriptionByID(pctx, id); err != nil {
		s.logger.Error("webpush: prune subscription failed",
			slog.String("subscription_id", id), slog.String("error", err.Error()))
	}
}
