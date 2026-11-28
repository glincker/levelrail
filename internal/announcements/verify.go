package announcements

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/advisoryfeed"
)

// Signature is one detached signature entry in the .sig file.
type Signature struct {
	KeyID string `json:"key_id"`
	Sig   string `json:"sig"`
}

// Verification errors. Callers keep the cached feed on any of them.
var (
	ErrNoTrustedKeys = errors.New("announcements: no trusted keys configured")
	ErrBadSignature  = errors.New("announcements: no valid signature from a trusted key")
	ErrReplay        = errors.New("announcements: feed sequence is older than the last accepted one")
	ErrUnchanged     = errors.New("announcements: feed sequence already accepted")
	ErrFeedExpired   = errors.New("announcements: feed has expired")
	ErrFeedLifetime  = errors.New("announcements: feed claims a lifetime beyond the allowed maximum")
)

// ParseSignatures decodes the detached signature list.
func ParseSignatures(b []byte) ([]Signature, error) {
	var sigs []Signature
	if err := json.Unmarshal(b, &sigs); err != nil {
		return nil, fmt.Errorf("announcements: malformed signature file: %w", err)
	}
	return sigs, nil
}

// Rejection records why one item was not accepted.
type Rejection struct {
	ID     string
	Reason string
}

// VerifyInput is everything VerifyFeed needs; it does no I/O.
type VerifyInput struct {
	Data         []byte
	SigData      []byte
	Keys         []Key
	Revoked      map[string]bool
	LastSequence int64
	Now          time.Time
	Limits       Limits
}

// Verified is an authenticated feed: Items passed the signature tier rules
// and the content rules, Revoked lists keys a critical revoke_key item named.
type Verified struct {
	Feed     Feed
	Signers  []string
	Items    []Item
	Revoked  []string
	Rejected []Rejection
}

// VerifyFeed authenticates feed bytes against trusted keys, then applies
// sequence, expiry, tier and content rules. Any whole-feed failure returns an
// error; per-item failures only drop that item.
func VerifyFeed(in VerifyInput) (Verified, error) {
	keys := map[string]Key{}
	for _, k := range in.Keys {
		if !in.Revoked[k.ID] {
			keys[k.ID] = k
		}
	}
	if len(keys) == 0 {
		return Verified{}, ErrNoTrustedKeys
	}
	if in.Limits.MaxBytes > 0 && int64(len(in.Data)) > in.Limits.MaxBytes {
		return Verified{}, ErrTooLarge
	}
	sigs, err := ParseSignatures(in.SigData)
	if err != nil {
		return Verified{}, err
	}
	signers := validSigners(in.Data, sigs, keys)
	if len(signers) == 0 {
		return Verified{}, ErrBadSignature
	}
	feed, err := ParseFeed(in.Data, in.Limits)
	if err != nil {
		return Verified{}, err
	}
	if err := checkFeedWindow(feed, in); err != nil {
		return Verified{}, err
	}
	out := Verified{Feed: feed, Signers: signers}
	for _, it := range feed.Items {
		if !it.ExpiresAt.After(in.Now) {
			continue
		}
		if err := acceptItem(it, signers, keys, in.Limits); err != nil {
			out.Rejected = append(out.Rejected, Rejection{ID: it.ID, Reason: err.Error()})
			continue
		}
		if it.Kind == KindRevokeKey {
			out.Revoked = append(out.Revoked, it.KeyID)
			continue
		}
		out.Items = append(out.Items, it)
	}
	return out, nil
}

func validSigners(data []byte, sigs []Signature, keys map[string]Key) []string {
	seen := map[string]bool{}
	for _, s := range sigs {
		k, ok := keys[s.KeyID]
		if !ok || seen[s.KeyID] {
			continue
		}
		if advisoryfeed.Verify(k.Public, data, []byte(s.Sig)) == nil {
			seen[s.KeyID] = true
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func checkFeedWindow(feed Feed, in VerifyInput) error {
	switch {
	case feed.Sequence < in.LastSequence:
		return ErrReplay
	case feed.Sequence == in.LastSequence:
		return ErrUnchanged
	case !feed.ExpiresAt.After(in.Now.Add(-in.Limits.ClockSkew)):
		return ErrFeedExpired
	case in.Limits.MaxAge > 0 && feed.ExpiresAt.After(in.Now.Add(in.Limits.MaxAge+in.Limits.ClockSkew)):
		return ErrFeedLifetime
	}
	return nil
}

// acceptItem requires enough distinct signers allowed for the item's tier,
// then validates content, so a key cannot sign above its tier limit.
func acceptItem(it Item, signers []string, keys map[string]Key, lim Limits) error {
	allowed := 0
	for _, id := range signers {
		if keys[id].CanSign(it.Tier) {
			allowed++
		}
	}
	if need := RequiredSignatures(it.Tier); allowed < need {
		return fmt.Errorf("tier %s needs %d allowed signatures, has %d", it.Tier, need, allowed)
	}
	return ValidateItem(it, lim)
}
