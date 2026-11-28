// Package announcements verifies, validates and caches the signed
// announcement feed (security notices, product news, newsletter). The feature
// is inert until the binary embeds trusted public keys (keys.go), and every
// check fails closed: an item is shown only when signed by keys allowed for
// its tier.
package announcements

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// SchemaVersion is the only feed schema this build reads; a breaking change
// ships as a parallel feed file, so old instances keep reading v1.
const SchemaVersion = 1

// Tiers, from most to least urgent.
const (
	TierCritical    = "critical"
	TierAdvisory    = "advisory"
	TierMaintenance = "maintenance"
	TierFeature     = "feature"
	TierInfo        = "info"
	TierNewsletter  = "newsletter"
)

// Item kinds. A revoke_key item names a signing key id to stop trusting.
const (
	KindAnnouncement = "announcement"
	KindRevokeKey    = "revoke_key"
)

// Allowed call-to-action routes, never URLs.
const (
	CTAUpdates  = "updates"
	CTASettings = "settings"
)

var knownTiers = map[string]bool{
	TierCritical: true, TierAdvisory: true, TierMaintenance: true,
	TierFeature: true, TierInfo: true, TierNewsletter: true,
}

// Link is one outbound link, validated against the host allowlist.
type Link struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

// Item is one announcement. Unknown JSON fields are ignored so a newer feed
// still parses on an older binary.
type Item struct {
	ID        string    `json:"id"`
	Tier      string    `json:"tier"`
	Kind      string    `json:"kind"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	AppliesTo string    `json:"applies_to,omitempty"`
	Channels  []string  `json:"channels,omitempty"`
	CTA       string    `json:"cta,omitempty"`
	Links     []Link    `json:"links,omitempty"`
	KeyID     string    `json:"key_id,omitempty"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Feed is the parsed envelope. Dropped counts items skipped as malformed or
// of an unknown tier or kind; the rest of the feed is kept.
type Feed struct {
	Schema    int       `json:"schema"`
	Sequence  int64     `json:"sequence"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Items     []Item    `json:"-"`
	Dropped   int       `json:"-"`
}

// Limits caps feed size and field lengths. Values come from runtime settings.
type Limits struct {
	MaxBytes  int64
	MaxItems  int
	MaxTitle  int
	MaxBody   int
	MaxAge    time.Duration
	ClockSkew time.Duration
}

// Parse errors.
var (
	ErrTooLarge    = errors.New("announcements: feed exceeds the size cap")
	ErrTooManyItem = errors.New("announcements: feed exceeds the item cap")
	ErrSchema      = errors.New("announcements: unsupported feed schema")
)

type envelope struct {
	Schema    int               `json:"schema"`
	Sequence  int64             `json:"sequence"`
	IssuedAt  time.Time         `json:"issued_at"`
	ExpiresAt time.Time         `json:"expires_at"`
	Items     []json.RawMessage `json:"items"`
}

// ParseFeed decodes feed bytes tolerantly: unknown fields are ignored and a
// malformed or unknown-tier item is dropped without failing the feed.
func ParseFeed(data []byte, lim Limits) (Feed, error) {
	if lim.MaxBytes > 0 && int64(len(data)) > lim.MaxBytes {
		return Feed{}, ErrTooLarge
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return Feed{}, fmt.Errorf("announcements: malformed feed: %w", err)
	}
	if env.Schema != SchemaVersion {
		return Feed{}, ErrSchema
	}
	if env.Sequence <= 0 {
		return Feed{}, errors.New("announcements: feed has no positive sequence")
	}
	if lim.MaxItems > 0 && len(env.Items) > lim.MaxItems {
		return Feed{}, ErrTooManyItem
	}
	feed := Feed{Schema: env.Schema, Sequence: env.Sequence, IssuedAt: env.IssuedAt, ExpiresAt: env.ExpiresAt}
	seen := map[string]bool{}
	for _, raw := range env.Items {
		var it Item
		if err := json.Unmarshal(raw, &it); err != nil || !knownTiers[it.Tier] || seen[it.ID] {
			feed.Dropped++
			continue
		}
		if it.Kind == "" {
			it.Kind = KindAnnouncement
		}
		if it.Kind != KindAnnouncement && it.Kind != KindRevokeKey {
			feed.Dropped++
			continue
		}
		seen[it.ID] = true
		feed.Items = append(feed.Items, it)
	}
	return feed, nil
}
