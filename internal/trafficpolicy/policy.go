// Package trafficpolicy holds the per-domain traffic control model (headers,
// path forwarders, geolocation, caching, redirects): the wire types the API,
// CLI and dashboard share, their validation, and a human readable preview.
// It has no dependencies outside the standard library so the ingress, store
// and API layers can all import it.
package trafficpolicy

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Policy kinds, one stored row each per domain.
const (
	KindHeaders    = "headers"
	KindForwarders = "forwarders"
	KindGeo        = "geo"
	KindCache      = "cache"
	KindRedirects  = "redirects"
)

// Kinds lists every policy kind in evaluation order.
var Kinds = []string{KindRedirects, KindGeo, KindHeaders, KindCache, KindForwarders}

// IsKind reports whether k names a policy kind.
func IsKind(k string) bool {
	for _, known := range Kinds {
		if k == known {
			return true
		}
	}
	return false
}

// Policy is every traffic control configured for one domain. A nil section
// means the domain uses the default behavior for it.
type Policy struct {
	Headers    *Headers    `json:"headers,omitempty"`
	Forwarders *Forwarders `json:"forwarders,omitempty"`
	Geo        *Geo        `json:"geo,omitempty"`
	Cache      *Cache      `json:"cache,omitempty"`
	Redirects  *Redirects  `json:"redirects,omitempty"`
}

// Empty reports whether no section is configured.
func (p Policy) Empty() bool {
	return p.Headers == nil && p.Forwarders == nil && p.Geo == nil && p.Cache == nil && p.Redirects == nil
}

// FieldError is one validation failure, addressed by a dotted field path
// (for example "rules[2].name") so the dashboard can show it inline.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError collects every FieldError found in one section.
type ValidationError struct {
	Kind   string       `json:"kind"`
	Fields []FieldError `json:"fields"`
}

func (e *ValidationError) Error() string {
	if len(e.Fields) == 0 {
		return e.Kind + ": invalid"
	}
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		if f.Field == "" {
			parts = append(parts, f.Message)
			continue
		}
		parts = append(parts, f.Field+": "+f.Message)
	}
	return e.Kind + ": " + strings.Join(parts, "; ")
}

type collector struct {
	kind   string
	fields []FieldError
}

func (c *collector) add(field, format string, args ...any) {
	c.fields = append(c.fields, FieldError{Field: field, Message: fmt.Sprintf(format, args...)})
}

func (c *collector) err() error {
	if len(c.fields) == 0 {
		return nil
	}
	return &ValidationError{Kind: c.kind, Fields: c.fields}
}

// AsValidationError unwraps err into a ValidationError when it is one.
func AsValidationError(err error) (*ValidationError, bool) {
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve, true
	}
	return nil, false
}

// Context is what validation needs to know about the domain and instance.
type Context struct {
	// Domain is the host the policy belongs to, lower case.
	Domain string
	// TLSReal is true when browsers see a publicly trusted certificate for
	// Domain (ACME, an uploaded certificate, or TLS terminated upstream).
	TLSReal bool
	// AppExists reports whether a forwarder's target app exists. Nil skips
	// the check.
	AppExists func(name string) bool
}

// Decode parses the stored JSON for kind into the matching section of p.
// On error p is left unchanged.
func (p *Policy) Decode(kind string, raw []byte) error {
	var err error
	switch kind {
	case KindHeaders:
		p.Headers, err = decodeSection(p.Headers, kind, raw)
	case KindForwarders:
		p.Forwarders, err = decodeSection(p.Forwarders, kind, raw)
	case KindGeo:
		p.Geo, err = decodeSection(p.Geo, kind, raw)
	case KindCache:
		p.Cache, err = decodeSection(p.Cache, kind, raw)
	case KindRedirects:
		p.Redirects, err = decodeSection(p.Redirects, kind, raw)
	default:
		return fmt.Errorf("trafficpolicy: unknown kind %q", kind)
	}
	return err
}

func decodeSection[T any](current *T, kind string, raw []byte) (*T, error) {
	v := new(T)
	if err := json.Unmarshal(raw, v); err != nil {
		return current, fmt.Errorf("trafficpolicy: decode %s: %w", kind, err)
	}
	return v, nil
}

// Validate checks every configured section.
func (p Policy) Validate(l Limits, c Context) error {
	var errs []error
	if p.Headers != nil {
		errs = append(errs, p.Headers.Validate(l, c))
	}
	if p.Forwarders != nil {
		errs = append(errs, p.Forwarders.Validate(l, c))
	}
	if p.Geo != nil {
		errs = append(errs, p.Geo.Validate(l))
	}
	if p.Cache != nil {
		errs = append(errs, p.Cache.Validate(l))
	}
	if p.Redirects != nil {
		errs = append(errs, p.Redirects.Validate())
	}
	return errors.Join(errs...)
}
