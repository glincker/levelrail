package spec

import (
	"fmt"
	"strconv"
	"strings"
)

// Env names and defaults for the Pages password gate length bounds. bcrypt
// ignores bytes past 72, so the maximum is capped there whatever the env says.
const (
	//nolint:gosec // env var name, not a credential
	EnvPagesPasswordMin = "APP_PAGES_PASSWORD_MIN"
	//nolint:gosec // env var name, not a credential
	EnvPagesPasswordMax = "APP_PAGES_PASSWORD_MAX"

	defaultPagesPasswordMin = 8
	defaultPagesPasswordMax = 72
)

func pagesPasswordBound(lookup func(string) (string, bool), key string, def int) int {
	if v, ok := lookup(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// ValidatePagesPassword checks a site gate password against the env bounds.
func ValidatePagesPassword(pw string, lookup func(string) (string, bool)) error {
	lo := pagesPasswordBound(lookup, EnvPagesPasswordMin, defaultPagesPasswordMin)
	hi := min(pagesPasswordBound(lookup, EnvPagesPasswordMax, defaultPagesPasswordMax), 72)
	if len(pw) < lo || len(pw) > hi {
		return fmt.Errorf("password must be %d to %d bytes", lo, hi)
	}
	if strings.ContainsAny(pw, "\r\n\x00") {
		return fmt.Errorf("password must be a single line")
	}
	return nil
}
