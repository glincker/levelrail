package secretsource

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

// Mask replaces every registered value in output.
const Mask = "[redacted]"

// Redactor masks known secret values in strings and errors. Safe for
// concurrent use.
type Redactor struct {
	mu   sync.RWMutex
	vals []string // longest first so a prefix value cannot expose a longer one
}

// NewRedactor returns an empty Redactor.
func NewRedactor() *Redactor { return &Redactor{} }

// Add registers a value. Empty values are ignored.
func (r *Redactor) Add(value []byte) {
	if len(value) == 0 {
		return
	}
	s := string(value)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range r.vals {
		if v == s {
			return
		}
	}
	r.vals = append(r.vals, s)
	sort.Slice(r.vals, func(i, j int) bool { return len(r.vals[i]) > len(r.vals[j]) })
}

// Mask returns s with every registered value replaced.
func (r *Redactor) Mask(s string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, v := range r.vals {
		s = strings.ReplaceAll(s, v, Mask)
	}
	return s
}

// MaskError keeps the error chain for errors.Is but masks the message.
func (r *Redactor) MaskError(err error) error {
	if err == nil {
		return nil
	}
	return &maskedError{msg: r.Mask(err.Error()), err: err}
}

type maskedError struct {
	msg string
	err error
}

func (e *maskedError) Error() string { return e.msg }
func (e *maskedError) Unwrap() error { return e.err }

// IsKind reports whether err is one of the package sentinels.
func IsKind(err error) bool {
	for _, k := range []error{ErrInvalidRef, ErrNotFound, ErrDenied, ErrUnavailable, ErrFailClosed, ErrUnknownConnection, ErrUnsupported} {
		if errors.Is(err, k) {
			return true
		}
	}
	return false
}
