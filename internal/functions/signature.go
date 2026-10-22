package functions

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Headers a webhook caller sends: a unix timestamp and sha256=hex(HMAC(secret, timestamp + "." + body)).
const (
	HeaderTimestamp = "X-Signature-Timestamp"
	HeaderSignature = "X-Signature"
)

// Errors returned by Verify.
var (
	ErrSignatureMissing = errors.New("functions: signature headers missing")
	ErrSignatureStale   = errors.New("functions: signature timestamp outside the replay window")
	ErrSignatureInvalid = errors.New("functions: signature does not match")
)

// Sign returns the signature header value for body at the given unix timestamp.
func Sign(secret string, timestamp int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(timestamp, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Verify checks the timestamp is within window of now and the signature matches. It returns the signature for replay dedupe.
func Verify(secret, tsHeader, sigHeader string, body []byte, window time.Duration, now time.Time) (string, error) {
	if tsHeader == "" || sigHeader == "" {
		return "", ErrSignatureMissing
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(tsHeader), 10, 64)
	if err != nil {
		return "", fmt.Errorf("%w: bad timestamp", ErrSignatureInvalid)
	}
	if d := now.Sub(time.Unix(sec, 0)); d > window || d < -window {
		return "", ErrSignatureStale
	}
	want := Sign(secret, sec, body)
	if !hmac.Equal([]byte(want), []byte(strings.TrimSpace(sigHeader))) {
		return "", ErrSignatureInvalid
	}
	return want, nil
}

// NewSecret returns a random webhook secret.
func NewSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("functions: generate webhook secret: %w", err)
	}
	return "whsec_" + hex.EncodeToString(b), nil
}

// NewInvocationID returns a random invocation id.
func NewInvocationID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("functions: generate invocation id: %w", err)
	}
	return "inv_" + hex.EncodeToString(b), nil
}
