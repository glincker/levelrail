package secrets

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// WrappedPrefix marks a master key file or env value that an external key
// service wrapped, so loading it never falls back to treating it as an age identity.
const WrappedPrefix = "levelrail-wrapped-v1:"

// ErrWrapperRequired means a wrapped master key was found but no key service is configured.
var ErrWrapperRequired = errors.New("secrets: master key is wrapped by an external key service but none is configured")

// KeyWrapper seals and opens the serialized master key with an external key
// service, so the key at rest is useless without that service.
type KeyWrapper interface {
	// Name identifies the backend inside the wrapped blob.
	Name() string
	Wrap(ctx context.Context, plaintext []byte) (string, error)
	Unwrap(ctx context.Context, ciphertext string) ([]byte, error)
}

// IsWrappedMasterKey reports whether s is a wrapped blob rather than a plain identity.
func IsWrappedMasterKey(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), WrappedPrefix)
}

// WrapMasterKey seals a serialized master key as WrappedPrefix + backend name + ciphertext.
func WrapMasterKey(ctx context.Context, serialized string, w KeyWrapper) (string, error) {
	if w == nil {
		return "", ErrWrapperRequired
	}
	ct, err := w.Wrap(ctx, []byte(strings.TrimSpace(serialized)))
	if err != nil {
		return "", fmt.Errorf("secrets: wrap master key with %s: %w", w.Name(), err)
	}
	return WrappedPrefix + w.Name() + ":" + ct, nil
}

// UnwrapMasterKey opens a blob made by WrapMasterKey and returns the serialized
// master key. It refuses a blob that a different backend sealed.
func UnwrapMasterKey(ctx context.Context, blob string, w KeyWrapper) (string, error) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(blob), WrappedPrefix)
	if !ok {
		return "", errors.New("secrets: value is not a wrapped master key")
	}
	if w == nil {
		return "", ErrWrapperRequired
	}
	name, ct, ok := strings.Cut(rest, ":")
	if !ok || ct == "" {
		return "", errors.New("secrets: wrapped master key is malformed")
	}
	if name != w.Name() {
		return "", fmt.Errorf("secrets: master key was wrapped by %q but %q is configured", name, w.Name())
	}
	pt, err := w.Unwrap(ctx, ct)
	if err != nil {
		return "", fmt.Errorf("secrets: unwrap master key with %s: %w", name, err)
	}
	return strings.TrimSpace(string(pt)), nil
}
