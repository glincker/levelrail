package certflow

import (
	"fmt"
	"strings"
)

// CodeReplaces is the preflight item and API error code for a staging request
// that would replace a working certificate.
const CodeReplaces = "replaces_certificate"

// Existing is a stored certificate as the staging guard sees it.
type Existing struct {
	Domain  string
	Kind    Kind
	Expired bool
	Stalled bool
}

// usable is true for a certificate that still works for visitors: not
// expired and not stuck. An expired or stalled one may be replaced.
func (e Existing) usable() bool { return !e.Expired && !e.Stalled }

// ReplaceError says a staging request would swap a working certificate for an
// untrusted one.
type ReplaceError struct {
	Domain string
	Kind   Kind
}

// Error is the plain-language message shown by the API, CLI and dashboard.
func (e *ReplaceError) Error() string {
	return fmt.Sprintf("Staging would replace the working %s certificate for %s with one browsers do not trust. Staging is only for a domain still on the internal certificate or on a staging certificate. Next step: request the real certificate without staging.", kindWord(e.Kind), e.Domain)
}

func kindWord(k Kind) string {
	switch k {
	case KindLetsEncrypt:
		return "Let's Encrypt"
	case KindCustom:
		return "uploaded"
	case KindProxy:
		return "proxy"
	}
	return "public CA"
}

// StagingConflict returns the first working certificate a staging request
// would put at risk, or nil. Switching the CA drops every stored ACME
// certificate server-wide, so a working Let's Encrypt or public CA
// certificate on any domain counts; an uploaded or proxy certificate counts
// only for the requested domain.
func StagingConflict(domain string, existing []Existing) *ReplaceError {
	for _, e := range existing {
		if !e.usable() {
			continue
		}
		switch e.Kind {
		case KindLetsEncrypt, KindOther:
			return &ReplaceError{Domain: e.Domain, Kind: e.Kind}
		case KindCustom, KindProxy:
			if strings.EqualFold(e.Domain, domain) {
				return &ReplaceError{Domain: e.Domain, Kind: e.Kind}
			}
		}
	}
	return nil
}
