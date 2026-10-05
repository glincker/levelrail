package authengine

import (
	"fmt"
	"os"
	"strings"
)

// Area is a slice of auth behavior that can move to the library on its own.
type Area string

const (
	// AreaTokens covers bearer API token authentication and token management.
	AreaTokens Area = "tokens"
	// AreaDevice covers the device login flow.
	AreaDevice Area = "device"
	// AreaSessions covers password login, sessions, logout, setup token and account recovery.
	AreaSessions Area = "sessions"
	// AreaMFA covers TOTP and passkeys.
	AreaMFA Area = "mfa"
	// AreaOAuth covers OAuth and OIDC sign-in.
	AreaOAuth Area = "oauth"

	// EnvAreas lists the areas served by the library, comma separated. Empty means every area.
	EnvAreas = "APP_AUTH_ENGINE_AREAS"
)

// AllAreas is every Area in cutover order.
var AllAreas = []Area{AreaTokens, AreaDevice, AreaSessions, AreaMFA, AreaOAuth}

// ParseAreas turns a comma separated list into Areas. An empty list means all areas.
func ParseAreas(raw string) ([]Area, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return append([]Area(nil), AllAreas...), nil
	}
	known := make(map[Area]bool, len(AllAreas))
	for _, a := range AllAreas {
		known[a] = true
	}
	var out []Area
	seen := make(map[Area]bool)
	for _, part := range strings.Split(raw, ",") {
		a := Area(strings.ToLower(strings.TrimSpace(part)))
		if a == "" {
			continue
		}
		if !known[a] {
			return nil, fmt.Errorf("authengine: unknown area %q in %s (valid: %v)", a, EnvAreas, AllAreas)
		}
		if !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out, nil
}

// ValidateAreas fails when APP_AUTH_ENGINE_AREAS names an unknown area; call it at startup.
func ValidateAreas() error {
	_, err := ParseAreas(os.Getenv(EnvAreas))
	return err
}

// AreaActive reports whether the library serves area a: the engine mode is
// library and a is listed in APP_AUTH_ENGINE_AREAS (an empty list means all).
func AreaActive(a Area) bool {
	if !Enabled() {
		return false
	}
	areas, err := ParseAreas(os.Getenv(EnvAreas))
	if err != nil {
		return false
	}
	for _, x := range areas {
		if x == a {
			return true
		}
	}
	return false
}
