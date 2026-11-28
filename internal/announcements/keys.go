package announcements

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"
)

// Key is one trusted announcement signing key. Tiers limits what it may sign:
// a signature from a key counts only for items whose tier it lists.
type Key struct {
	ID     string
	Public ed25519.PublicKey
	Tiers  []string
}

// productionKeys is deliberately EMPTY. The owner adds the newsletter key
// (tiers newsletter, feature, maintenance, info) and the two offline security
// keys (tiers advisory, critical) here in a release. Until then the feature is
// inert: no network fetch happens and no item is ever accepted.
//
// PLACEHOLDER: no production key has been generated or embedded.
var productionKeys = []Key{}

// ProductionKeys returns a copy of the embedded trusted keys.
func ProductionKeys() []Key {
	return slices.Clone(productionKeys)
}

// RequiredSignatures is how many distinct trusted keys must sign a tier.
// Critical needs two, so one leaked security key cannot raise a critical alarm.
func RequiredSignatures(tier string) int {
	if tier == TierCritical {
		return 2
	}
	return 1
}

// CanSign reports whether the key may sign the tier.
func (k Key) CanSign(tier string) bool {
	return slices.Contains(k.Tiers, tier)
}

// ParseDevKeys decodes "id:base64pubkey:tier|tier" entries. It is used only
// for test and dev instances, see EffectiveKeys.
func ParseDevKeys(entries []string) ([]Key, error) {
	var out []Key
	for _, e := range entries {
		parts := strings.Split(strings.TrimSpace(e), ":")
		if len(parts) != 3 || parts[0] == "" {
			return nil, fmt.Errorf("announcements: dev key %q must be id:base64key:tier|tier", e)
		}
		raw, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("announcements: dev key %q is not a 32 byte ed25519 key", parts[0])
		}
		var tiers []string
		for _, t := range strings.Split(parts[2], "|") {
			if !knownTiers[t] {
				return nil, fmt.Errorf("announcements: dev key %q names unknown tier %q", parts[0], t)
			}
			tiers = append(tiers, t)
		}
		out = append(out, Key{ID: parts[0], Public: ed25519.PublicKey(raw), Tiers: tiers})
	}
	return out, nil
}

// EffectiveKeys returns the keys to trust. Extra keys from the runtime setting
// are honoured only in dev mode; in production mode they are ignored and the
// second result reports that, so a stray setting can never widen trust.
func EffectiveKeys(devMode bool, extra []string) (keys []Key, ignoredExtra bool, err error) {
	keys = ProductionKeys()
	if len(extra) == 0 {
		return keys, false, nil
	}
	if !devMode {
		return keys, true, nil
	}
	dev, err := ParseDevKeys(extra)
	if err != nil {
		return keys, false, err
	}
	return append(keys, dev...), false, nil
}
