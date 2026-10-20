// Package timeutil parses the RFC 3339 timestamps stored as text.
package timeutil

import "time"

// Parse parses an RFC 3339 timestamp with or without fractional seconds.
func Parse(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

// ParseOrZero returns the zero time when s is empty or malformed.
func ParseOrZero(s string) time.Time {
	t, err := Parse(s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// ParseOptional returns nil for an empty s and an error for a malformed one.
func ParseOptional(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := Parse(s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
