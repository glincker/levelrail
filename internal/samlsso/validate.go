package samlsso

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/crewjam/saml"
)

// MaxResponseBytes caps the base64 SAMLResponse form value.
const MaxResponseBytes = 512 << 10

const bearerMethod = "urn:oasis:names:tc:SAML:2.0:cm:bearer" //nolint:gosec // SAML confirmation method URI, not a credential

// ErrInvalidResponse wraps every rejection; the wrapped detail is for logs only.
var ErrInvalidResponse = errors.New("samlsso: invalid saml response")

// Attributes names the assertion attributes to read; empty fields fall back
// to the common Okta, Entra ID and Keycloak names.
type Attributes struct {
	Email  string
	Name   string
	Groups string
}

var (
	defaultEmailAttrs = []string{"email", "mail", "emailaddress", "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress", "urn:oid:0.9.2342.19200300.100.1.3"}
	defaultNameAttrs  = []string{"displayName", "name", "http://schemas.microsoft.com/identity/claims/displayname", "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name", "cn"}
	defaultGroupAttrs = []string{"groups", "memberOf", "http://schemas.microsoft.com/ws/2008/06/identity/claims/groups"}
)

// Identity is what a validated assertion says about the user.
type Identity struct {
	NameID      string
	Email       string
	DisplayName string
	Groups      []string
	AssertionID string
	// ReplayUntil is how long the assertion ID must be remembered to refuse a replay.
	ReplayUntil time.Time
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidResponse, fmt.Sprintf(format, args...))
}

// Validate checks a base64 SAMLResponse posted to the ACS: signature against
// the IDP certificates, issuer, destination, recipient, audience, time
// windows and InResponseTo against requestID. IDP-initiated responses are refused.
func (s SP) Validate(samlResponse, requestID string, names Attributes) (id Identity, err error) {
	if requestID == "" {
		return Identity{}, invalid("no outstanding authn request")
	}
	if samlResponse == "" || len(samlResponse) > MaxResponseBytes {
		return Identity{}, invalid("response missing or too large")
	}
	raw, err := base64.StdEncoding.DecodeString(whitespace.ReplaceAllString(samlResponse, ""))
	if err != nil {
		return Identity{}, invalid("response is not base64")
	}
	sp, err := s.serviceProvider(true)
	if err != nil {
		return Identity{}, err
	}
	acs, _ := url.Parse(s.ACSURL)
	defer func() {
		if p := recover(); p != nil {
			id, err = Identity{}, invalid("malformed assertion: %v", p)
		}
	}()
	assertion, perr := sp.ParseXMLResponse(raw, []string{requestID}, *acs)
	if perr != nil {
		var ire *saml.InvalidResponseError
		if errors.As(perr, &ire) && ire.PrivateErr != nil {
			return Identity{}, invalid("%v", ire.PrivateErr)
		}
		return Identity{}, invalid("%v", perr)
	}
	until, err := checkAssertion(assertion, requestID, s.ACSURL, saml.TimeNow())
	if err != nil {
		return Identity{}, err
	}
	return identityFrom(assertion, names, until)
}

// checkAssertion closes the gaps crewjam leaves open: it skips subject checks
// when there are no SubjectConfirmations and treats NameID as optional.
func checkAssertion(a *saml.Assertion, requestID, acs string, now time.Time) (time.Time, error) {
	if a.ID == "" {
		return time.Time{}, invalid("assertion has no ID")
	}
	if a.Subject == nil || a.Subject.NameID == nil || strings.TrimSpace(a.Subject.NameID.Value) == "" {
		return time.Time{}, invalid("assertion has no subject NameID")
	}
	if a.Conditions == nil || a.Conditions.NotOnOrAfter.IsZero() {
		return time.Time{}, invalid("assertion has no NotOnOrAfter condition")
	}
	until := a.Conditions.NotOnOrAfter
	bearer := false
	for _, sc := range a.Subject.SubjectConfirmations {
		d := sc.SubjectConfirmationData
		if sc.Method != bearerMethod || d == nil {
			continue
		}
		if d.InResponseTo != requestID || d.Recipient != acs || d.NotOnOrAfter.IsZero() {
			return time.Time{}, invalid("bearer confirmation does not match this request")
		}
		if !d.NotOnOrAfter.Add(saml.MaxClockSkew).After(now) {
			return time.Time{}, invalid("bearer confirmation expired")
		}
		if d.NotOnOrAfter.After(until) {
			until = d.NotOnOrAfter
		}
		bearer = true
	}
	if !bearer {
		return time.Time{}, invalid("assertion has no bearer subject confirmation")
	}
	return until.Add(saml.MaxClockSkew), nil
}

func identityFrom(a *saml.Assertion, names Attributes, until time.Time) (Identity, error) {
	attrs := map[string][]string{}
	for _, stmt := range a.AttributeStatements {
		for _, attr := range stmt.Attributes {
			for _, v := range attr.Values {
				val := strings.TrimSpace(v.Value)
				if val == "" {
					continue
				}
				attrs[strings.ToLower(attr.Name)] = append(attrs[strings.ToLower(attr.Name)], val)
				if attr.FriendlyName != "" {
					attrs[strings.ToLower(attr.FriendlyName)] = append(attrs[strings.ToLower(attr.FriendlyName)], val)
				}
			}
		}
	}
	nameID := strings.TrimSpace(a.Subject.NameID.Value)
	email := first(lookup(attrs, names.Email, defaultEmailAttrs))
	if email == "" && a.Subject.NameID.Format == string(saml.EmailAddressNameIDFormat) {
		email = nameID
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email != "" {
		addr, err := mail.ParseAddress(email)
		if err != nil || addr.Address != email {
			return Identity{}, invalid("email attribute is not an address")
		}
	}
	return Identity{
		NameID:      nameID,
		Email:       email,
		DisplayName: first(lookup(attrs, names.Name, defaultNameAttrs)),
		Groups:      lookup(attrs, names.Groups, defaultGroupAttrs),
		AssertionID: a.ID,
		ReplayUntil: until,
	}, nil
}

// lookup reads one configured attribute, or the first default present.
func lookup(attrs map[string][]string, configured string, defaults []string) []string {
	if configured != "" {
		return attrs[strings.ToLower(configured)]
	}
	for _, d := range defaults {
		if v := attrs[strings.ToLower(d)]; len(v) > 0 {
			return v
		}
	}
	return nil
}

func first(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
}
