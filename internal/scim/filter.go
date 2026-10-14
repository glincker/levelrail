// Package scim holds the SCIM 2.0 protocol pieces that need no storage:
// resource shapes, the filter subset and PATCH operation parsing.
package scim

import (
	"errors"
	"strings"
)

// ErrInvalidFilter is answered as 400 with scimType invalidFilter.
var ErrInvalidFilter = errors.New("scim: unsupported filter")

// Filter is the supported subset: one attribute compared with eq.
type Filter struct {
	Attribute string
	Value     string
}

// ParseFilter accepts `attr eq "value"` (RFC 7644 section 3.4.2.2), the form
// Okta, Entra ID and Keycloak send for lookups. Empty input means no filter.
// allowed lists the attribute names the endpoint understands, compared case-insensitively.
func ParseFilter(raw string, allowed ...string) (*Filter, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	attr, rest, ok := strings.Cut(raw, " ")
	if !ok {
		return nil, ErrInvalidFilter
	}
	op, value, ok := strings.Cut(strings.TrimSpace(rest), " ")
	if !ok || !strings.EqualFold(op, "eq") {
		return nil, ErrInvalidFilter
	}
	value = strings.TrimSpace(value)
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return nil, ErrInvalidFilter
	}
	value = strings.ReplaceAll(value[1:len(value)-1], `\"`, `"`)
	if strings.Contains(value, `"`) {
		return nil, ErrInvalidFilter
	}
	for _, a := range allowed {
		if strings.EqualFold(a, attr) {
			return &Filter{Attribute: a, Value: value}, nil
		}
	}
	return nil, ErrInvalidFilter
}
