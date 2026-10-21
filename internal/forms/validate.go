package forms

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
)

// DefaultHoneypot is the hidden field name used when a form sets none.
const DefaultHoneypot = "bot-field"

const maxRedirectLen = 2048

var (
	nameRE     = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
	honeypotRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
)

// ValidateName accepts lowercase letters, digits, dash and underscore, up to 63 characters.
func ValidateName(name string) error {
	if !nameRE.MatchString(name) {
		return errors.New("form name must be 1 to 63 lowercase letters, digits, dashes or underscores, starting with a letter or digit")
	}
	return nil
}

// ValidateRedirect accepts an empty value or an absolute http(s) URL without credentials.
func ValidateRedirect(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || len(raw) > maxRedirectLen || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return errors.New("redirect_url must be an absolute http or https URL without credentials")
	}
	return nil
}

// ValidateHoneypot accepts an empty value (the default) or a plain field name.
func ValidateHoneypot(field string) error {
	if field != "" && !honeypotRE.MatchString(field) {
		return errors.New("honeypot_field must start with a letter and use letters, digits, dashes or underscores")
	}
	return nil
}

// NewID returns a random form id.
func NewID() (string, error) {
	return randomHex("frm_", 12)
}

// NewSalt returns the random per-form salt used to hash submitter addresses.
func NewSalt() (string, error) {
	return randomHex("", 16)
}

func randomHex(prefix string, n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("forms: random id: %w", err)
	}
	return prefix + hex.EncodeToString(b), nil
}

// HashIP returns a short salted digest of ip; the raw address is never stored.
func HashIP(salt, ip string) string {
	if ip == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(salt + "|" + ip))
	return hex.EncodeToString(sum[:8])
}
