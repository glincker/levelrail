package webpush

import (
	"crypto/ecdh"
	"encoding/base64"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// EnvSendTimeout bounds one Send call, so a slow push service cannot stall the caller.
	EnvSendTimeout = "APP_WEBPUSH_SEND_TIMEOUT"
	// EnvAllowedHosts replaces the default push service host list (comma separated, "*.example.com" matches subdomains).
	EnvAllowedHosts = "APP_WEBPUSH_ALLOWED_HOSTS"
	// EnvMaxPerUser caps the browsers one account may register.
	EnvMaxPerUser = "APP_WEBPUSH_MAX_SUBSCRIPTIONS_PER_USER"

	defaultSendTimeout = 10 * time.Second
	defaultMaxPerUser  = 20
	sendConcurrency    = 8
)

// Field limits for a registration request.
const (
	MaxEndpointLen  = 2048
	MaxKeyLen       = 128
	MaxUserAgentLen = 512
	// MaxRequestBytes bounds the registration body.
	MaxRequestBytes = 8 << 10
)

var defaultAllowedHosts = []string{
	"fcm.googleapis.com",
	"updates.push.services.mozilla.com",
	"*.push.services.mozilla.com",
	"*.notify.windows.com",
	"web.push.apple.com",
	"*.push.apple.com",
}

// SendTimeout reads EnvSendTimeout, falling back to the default.
func SendTimeout(getenv func(string) string) time.Duration {
	if d, err := time.ParseDuration(strings.TrimSpace(getenv(EnvSendTimeout))); err == nil && d > 0 {
		return d
	}
	return defaultSendTimeout
}

// MaxPerUser reads EnvMaxPerUser, falling back to the default.
func MaxPerUser(getenv func(string) string) int {
	if n, err := strconv.Atoi(strings.TrimSpace(getenv(EnvMaxPerUser))); err == nil && n > 0 {
		return n
	}
	return defaultMaxPerUser
}

// AllowedHosts reads EnvAllowedHosts, falling back to the real push services.
func AllowedHosts(getenv func(string) string) []string {
	var out []string
	for _, h := range strings.Split(getenv(EnvAllowedHosts), ",") {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			out = append(out, h)
		}
	}
	if len(out) == 0 {
		return defaultAllowedHosts
	}
	return out
}

// ErrInvalidEndpoint is returned for an endpoint that is not an https URL on an allowed push service.
var ErrInvalidEndpoint = errors.New("webpush: endpoint is not an allowed push service address")

// ValidateEndpoint accepts only https URLs without credentials whose host matches allowed.
func ValidateEndpoint(endpoint string, allowed []string) error {
	if endpoint == "" || len(endpoint) > MaxEndpointLen {
		return ErrInvalidEndpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" {
		return ErrInvalidEndpoint
	}
	if p := u.Port(); p != "" && p != "443" {
		return ErrInvalidEndpoint
	}
	host := strings.ToLower(u.Hostname())
	for _, a := range allowed {
		if suffix, ok := strings.CutPrefix(a, "*."); ok {
			if strings.HasSuffix(host, "."+suffix) {
				return nil
			}
		} else if host == a {
			return nil
		}
	}
	return ErrInvalidEndpoint
}

// ErrInvalidKeys is returned for subscription keys of the wrong shape.
var ErrInvalidKeys = errors.New("webpush: invalid subscription keys")

const authSecretLen = 16

// ValidateKeys checks p256dh is an uncompressed P-256 point and auth is 16 bytes.
func ValidateKeys(p256dh, auth string) error {
	if len(p256dh) > MaxKeyLen || len(auth) > MaxKeyLen {
		return ErrInvalidKeys
	}
	pub, err := decodeKey(p256dh)
	if err != nil {
		return ErrInvalidKeys
	}
	if _, err := ecdh.P256().NewPublicKey(pub); err != nil {
		return ErrInvalidKeys
	}
	secret, err := decodeKey(auth)
	if err != nil || len(secret) != authSecretLen {
		return ErrInvalidKeys
	}
	return nil
}

// decodeKey accepts the unpadded URL-safe form browsers send, and padded or standard variants.
func decodeKey(s string) ([]byte, error) {
	s = strings.TrimRight(s, "=")
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}
