package announcements

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/glincker/levelrail/internal/advisoryfeed"
)

// Signature is one detached signature entry in the .sig file.
type Signature struct {
	KeyID string `json:"key_id"`
	Sig   string `json:"sig"`
}

// Verification errors. Callers keep the cached feed on any of them.
var (
	ErrNoTrustedKeys = errors.New("announcements: no trusted keys configured")
	ErrBadSignature  = errors.New("announcements: a signature from a trusted key does not verify")
	ErrNoSignature   = errors.New("announcements: no signature from a trusted key")
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

// Item drop codes, shared with the announce tool's verifier.
const (
	DropNotSigned = "tier_not_signed"
	DropExpired   = "expired"
	DropContent   = "content"
)

// Rejection records why one item was not accepted. Code is the shared drop
// code; Reason is the human detail.
type Rejection struct {
	ID     string
	Code   string
	Reason string
}

// ErrorCode maps a whole-feed failure to the code the announce tool uses, so
// both sides report the same outcome for the same bytes.
func ErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrTooLarge):
		return "too_large"
	case errors.Is(err, ErrBadSignature):
		return "bad_signature"
	case errors.Is(err, ErrNoSignature), errors.Is(err, ErrNoTrustedKeys):
		return "no_trusted_signature"
	case errors.Is(err, ErrSchema):
		return "bad_schema"
	case errors.Is(err, ErrReplay), errors.Is(err, ErrUnchanged):
		return "replay"
	case errors.Is(err, ErrFeedExpired):
		return "feed_expired"
	case errors.Is(err, ErrNotHTTPS):
		return "not_https"
	case errors.Is(err, ErrFetchStatus):
		return "fetch_status"
	case errors.Is(err, ErrFetch):
		return "fetch_failed"
	}
	return "bad_json"
}

// SafeErrorCode returns stored as-is when it is one of the stable codes, and
// "fetch_failed" for anything else, such as a raw error an older version stored.
func SafeErrorCode(stored string) string {
	switch stored {
	case "", "too_large", "bad_signature", "no_trusted_signature", "bad_schema", "replay",
		"feed_expired", "not_https", "fetch_status", "fetch_failed", "bad_json":
		return stored
	}
	return "fetch_failed"
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
	signers, err := validSigners(in.Data, sigs, keys)
	if err != nil {
		return Verified{}, err
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
		if code, err := acceptItem(it, signers, keys, in); err != nil {
			out.Rejected = append(out.Rejected, Rejection{ID: it.ID, Code: code, Reason: err.Error()})
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

// validSigners skips unknown key ids (rotation) but rejects the whole feed on
// a bad signature from a known key, the same rule the announce tool applies.
func validSigners(data []byte, sigs []Signature, keys map[string]Key) ([]string, error) {
	seen := map[string]bool{}
	for _, s := range sigs {
		k, ok := keys[s.KeyID]
		if !ok {
			continue
		}
		if advisoryfeed.Verify(k.Public, data, []byte(s.Sig)) != nil {
			return nil, ErrBadSignature
		}
		seen[s.KeyID] = true
	}
	if len(seen) == 0 {
		return nil, ErrNoSignature
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
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
// then expiry (with clock skew), then content, so a key cannot sign above its
// tier limit. The returned code is the shared drop code.
func acceptItem(it Item, signers []string, keys map[string]Key, in VerifyInput) (string, error) {
	allowed := 0
	for _, id := range signers {
		if keys[id].CanSign(it.Tier) {
			allowed++
		}
	}
	if need := RequiredSignatures(it.Tier); allowed < need {
		return DropNotSigned, fmt.Errorf("tier %s needs %d allowed signatures, has %d", it.Tier, need, allowed)
	}
	if !it.ExpiresAt.Add(in.Limits.ClockSkew).After(in.Now) {
		return DropExpired, errors.New("item has expired")
	}
	if err := ValidateItem(it, in.Limits); err != nil {
		return DropContent, err
	}
	return "", nil
}
