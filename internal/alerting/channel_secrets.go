package alerting

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ChannelSecrets is the envelope-encryption surface channel credentials
// are sealed with; *secrets.Manager satisfies it.
type ChannelSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// MaskedChannelSecret stands in for a sealed credential in every column
// and API response.
const MaskedChannelSecret = "(hidden)"

const (
	channelSecretNotifyURL   = "notify_url"
	channelSecretInteractive = "interactive_secret"
)

var (
	// ErrChannelSecretsUnavailable means a channel's credential is sealed
	// but no master key is configured to open it.
	ErrChannelSecretsUnavailable = errors.New("alerting: notification channel secrets need a master key")
	// ErrChannelSecretNotRevealed means a send was attempted with the
	// masked placeholder instead of the decrypted credential.
	ErrChannelSecretNotRevealed = errors.New("alerting: notification target is still masked")
)

// NotificationChannelSecretsKey is the internal/secrets serviceName a
// channel's credentials are stored under.
func NotificationChannelSecretsKey(id string) string { return "notification-channel/" + id }

// MaskNotifyTarget hides the credential in target, keeping only where it
// points. An email recipient is an address, not a credential.
func MaskNotifyTarget(kind NotifyKind, target string) string {
	if target == "" || kind == NotifyEmail {
		return target
	}
	if u, err := url.Parse(target); err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") {
		return u.Scheme + "://" + u.Host + "/" + MaskedChannelSecret
	}
	return MaskedChannelSecret
}

// IsMaskedNotifyTarget reports whether s is MaskNotifyTarget's output for
// a sealed credential rather than a usable destination.
func IsMaskedNotifyTarget(s string) bool {
	return s == MaskedChannelSecret || strings.HasSuffix(s, "/"+MaskedChannelSecret)
}

func kindSealsTarget(kind NotifyKind) bool {
	return kind != NotifyEmail && kind != NotifyWebpush
}

// SetChannelSecrets seals channel credentials from now on. nil keeps the
// plaintext fallback for a control plane with no master key.
func (db *DB) SetChannelSecrets(cs ChannelSecrets) {
	db.secretsMu.Lock()
	defer db.secretsMu.Unlock()
	db.channelSecrets = cs
}

func (db *DB) secretsOrNil() ChannelSecrets {
	if db == nil {
		return nil
	}
	db.secretsMu.RLock()
	defer db.secretsMu.RUnlock()
	return db.channelSecrets
}

// sealChannel stores c's credentials as ciphertext and returns the column
// values to persist in their place.
func (db *DB) sealChannel(ctx context.Context, cs ChannelSecrets, c NotificationChannel) (urlCol, secretCol string, err error) {
	key := NotificationChannelSecretsKey(c.ID)
	urlCol, secretCol = c.NotifyURL, c.InteractiveSecret
	if kindSealsTarget(c.Kind) && c.NotifyURL != "" && !IsMaskedNotifyTarget(c.NotifyURL) {
		if err := cs.SetValue(ctx, key, channelSecretNotifyURL, c.NotifyURL); err != nil {
			return "", "", fmt.Errorf("alerting: seal notify_url for %q: %w", c.ID, err)
		}
		urlCol = MaskNotifyTarget(c.Kind, c.NotifyURL)
	}
	if c.InteractiveSecret != "" && c.InteractiveSecret != MaskedChannelSecret {
		if err := cs.SetValue(ctx, key, channelSecretInteractive, c.InteractiveSecret); err != nil {
			return "", "", fmt.Errorf("alerting: seal interactive secret for %q: %w", c.ID, err)
		}
		secretCol = MaskedChannelSecret
	}
	return urlCol, secretCol, nil
}

// RevealNotificationChannel returns c with its sealed credentials
// decrypted. Call it only on the send or verify path, and never persist
// or return the result to a client.
func (db *DB) RevealNotificationChannel(ctx context.Context, c NotificationChannel) (NotificationChannel, error) {
	var err error
	if IsMaskedNotifyTarget(c.NotifyURL) {
		if c.NotifyURL, err = db.resolveChannelSecret(ctx, c.ID, channelSecretNotifyURL); err != nil {
			return NotificationChannel{}, err
		}
	}
	if c.InteractiveSecret == MaskedChannelSecret {
		if c.InteractiveSecret, err = db.resolveChannelSecret(ctx, c.ID, channelSecretInteractive); err != nil {
			return NotificationChannel{}, err
		}
	}
	return c, nil
}

// RevealChannelTarget decrypts target when it is a sealed channel's
// masked destination, as joined into a deploy target or alert rule.
func (db *DB) RevealChannelTarget(ctx context.Context, channelID, target string) (string, error) {
	if channelID == "" || !IsMaskedNotifyTarget(target) {
		return target, nil
	}
	return db.resolveChannelSecret(ctx, channelID, channelSecretNotifyURL)
}

func (db *DB) resolveChannelSecret(ctx context.Context, id, envKey string) (string, error) {
	cs := db.secretsOrNil()
	if cs == nil {
		return "", fmt.Errorf("channel %q: %w", id, ErrChannelSecretsUnavailable)
	}
	v, err := cs.Resolve(ctx, NotificationChannelSecretsKey(id), envKey)
	if err != nil {
		return "", fmt.Errorf("alerting: reveal %s for channel %q: %w", envKey, id, err)
	}
	return v, nil
}

func needsSealing(kind NotifyKind, notifyURL, secret string) bool {
	urlPlain := kindSealsTarget(kind) && notifyURL != "" && !IsMaskedNotifyTarget(notifyURL)
	secretPlain := secret != "" && secret != MaskedChannelSecret
	return urlPlain || secretPlain
}

type plaintextChannelRow struct {
	id, kind, notifyURL, secret string
}

func (db *DB) plaintextChannelRows(ctx context.Context) ([]plaintextChannelRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, kind, notify_url, interactive_secret FROM notification_channels ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("alerting: list channel credentials: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []plaintextChannelRow
	for rows.Next() {
		var r plaintextChannelRow
		if err := rows.Scan(&r.id, &r.kind, &r.notifyURL, &r.secret); err != nil {
			return nil, fmt.Errorf("alerting: scan channel credentials: %w", err)
		}
		if needsSealing(NotifyKind(r.kind), r.notifyURL, r.secret) {
			out = append(out, r)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("alerting: iterate channel credentials: %w", err)
	}
	return out, nil
}

// CountPlaintextNotificationChannels returns how many channels still
// hold a credential in plaintext.
func (db *DB) CountPlaintextNotificationChannels(ctx context.Context) (int, error) {
	rows, err := db.plaintextChannelRows(ctx)
	return len(rows), err
}

// SealNotificationChannelSecrets moves every plaintext channel credential
// under envelope encryption, verifying each by decrypting it before the
// plaintext column is cleared. Idempotent and safe to interrupt.
func (db *DB) SealNotificationChannelSecrets(ctx context.Context) (int, error) {
	cs := db.secretsOrNil()
	if cs == nil {
		return 0, ErrChannelSecretsUnavailable
	}
	rows, err := db.plaintextChannelRows(ctx)
	if err != nil {
		return 0, err
	}
	sealed := 0
	var errs []error
	for _, r := range rows {
		ok, err := db.sealRow(ctx, cs, r)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if ok {
			sealed++
		}
	}
	return sealed, errors.Join(errs...)
}

func (db *DB) sealRow(ctx context.Context, cs ChannelSecrets, r plaintextChannelRow) (bool, error) {
	c := NotificationChannel{ID: r.id, Kind: NotifyKind(r.kind), NotifyURL: r.notifyURL, InteractiveSecret: r.secret}
	urlCol, secretCol, err := db.sealChannel(ctx, cs, c)
	if err != nil {
		return false, err
	}
	sealedView := NotificationChannel{ID: r.id, NotifyURL: urlCol, InteractiveSecret: secretCol}
	revealed, err := db.RevealNotificationChannel(ctx, sealedView)
	if err != nil {
		return false, fmt.Errorf("alerting: verify sealed channel %q: %w", r.id, err)
	}
	if revealed.NotifyURL != r.notifyURL || revealed.InteractiveSecret != r.secret {
		return false, fmt.Errorf("alerting: verify sealed channel %q: decrypted value does not match", r.id)
	}
	// Compare-and-swap so a concurrent edit is never overwritten with a stale mask.
	res, err := db.ExecContext(ctx, `
		UPDATE notification_channels SET notify_url = ?, interactive_secret = ?
		WHERE id = ? AND notify_url = ? AND interactive_secret = ?`,
		urlCol, secretCol, r.id, r.notifyURL, r.secret)
	if err != nil {
		return false, fmt.Errorf("alerting: clear plaintext for channel %q: %w", r.id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("alerting: clear plaintext for channel %q: %w", r.id, err)
	}
	return n == 1, nil
}
