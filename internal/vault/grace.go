package vault

import (
	"fmt"
	"time"
)

// Attempt authenticates with one credential and reads the secret, returning the value.
type Attempt func(credential string) (string, error)

// ResolveWithGrace tries the current credential first. The previous credential
// is tried only while previousExpiresAt is set and still in the future, so a
// rotation grace of 0 (no previous kept, zero expiry) never falls back.
func ResolveWithGrace(now, previousExpiresAt time.Time, current, previous string, attempt Attempt) (string, error) {
	value, err := attempt(current)
	if err == nil {
		return value, nil
	}
	if previous == "" || previousExpiresAt.IsZero() || !now.Before(previousExpiresAt) {
		return "", err
	}
	value, prevErr := attempt(previous)
	if prevErr != nil {
		return "", fmt.Errorf("vault: previous credential rejected: %w", prevErr)
	}
	return value, nil
}
