package authengine

import (
	"encoding/base64"
	"encoding/json"
	"slices"
	"sort"
	"strings"
)

const (
	maxClaimGroups   = 200
	maxClaimGroupLen = 256
	maxIDTokenBytes  = 16 << 10
)

// IDTokenClaims is what a sign-in reads from an ID token the library already
// verified (signature, issuer, audience, nonce, expiry).
type IDTokenClaims struct {
	Groups []string
	// GroupsFound is true when the configured claim exists, even if empty.
	GroupsFound bool
	// GroupLikeClaims names other top-level claims that hold a list of
	// strings, so a wrong claim name can be corrected.
	GroupLikeClaims []string
}

// ReadIDTokenClaims decodes the payload of an ID token that the caller has
// verified. It never verifies anything itself: call it only on a token the
// library returned from a successful nonce exchange.
func ReadIDTokenClaims(idToken, groupsClaim string) IDTokenClaims {
	var out IDTokenClaims
	payload := idTokenPayload(idToken)
	if payload == nil {
		return out
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return out
	}
	groupsClaim = strings.TrimSpace(groupsClaim)
	if v, ok := claimAt(claims, groupsClaim); ok {
		out.GroupsFound = true
		out.Groups = claimStrings(v)
	}
	for k, v := range claims {
		if k == groupsClaim {
			continue
		}
		if list, ok := v.([]any); ok && len(claimStrings(list)) > 0 {
			out.GroupLikeClaims = append(out.GroupLikeClaims, k)
		}
	}
	sort.Strings(out.GroupLikeClaims)
	return out
}

func idTokenPayload(idToken string) []byte {
	if idToken == "" || len(idToken) > maxIDTokenBytes {
		return nil
	}
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	return b
}

// claimAt resolves a claim name, trying the literal key first so names that
// contain dots (URL style custom claims) work, then a dotted path.
func claimAt(claims map[string]any, name string) (any, bool) {
	if name == "" {
		return nil, false
	}
	if v, ok := claims[name]; ok {
		return v, true
	}
	var cur any = claims
	for _, part := range strings.Split(name, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = m[part]; !ok {
			return nil, false
		}
	}
	return cur, true
}

// claimStrings turns a claim value into trimmed, de-duplicated group names,
// bounded in count and length. A bare string is one group, never split.
func claimStrings(v any) []string {
	var raw []string
	switch x := v.(type) {
	case string:
		raw = []string{x}
	case []any:
		for _, e := range x {
			if s, ok := e.(string); ok {
				raw = append(raw, s)
			}
		}
	}
	out := make([]string, 0, len(raw))
	for _, g := range raw {
		g = strings.TrimSpace(g)
		if g == "" || len(g) > maxClaimGroupLen || slices.Contains(out, g) {
			continue
		}
		if len(out) == maxClaimGroups {
			break
		}
		out = append(out, g)
	}
	return out
}
