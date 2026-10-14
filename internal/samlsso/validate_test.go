package samlsso

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/crewjam/saml"

	"github.com/GLINCKER/levelrail/internal/samlsso/samltest"
)

func TestValidateAcceptsSignedResponse(t *testing.T) {
	idp := newTestIDP(t)
	sp := newTestSP(t, idp)
	for _, tc := range []struct {
		name string
		opts func(*samltest.Options)
	}{
		{"response and assertion signed", func(*samltest.Options) {}},
		{"assertion signed only", func(o *samltest.Options) { o.SignResponse = false }},
		{"response signed only", func(o *samltest.Options) { o.SignAssert = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := defaultOpts()
			tc.opts(&o)
			now := time.Now()
			got, err := sp.Validate(idp.Respond(t, goodAssertion(idp, now), o), testRequestID, Attributes{})
			if err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if got.NameID != "user-123" || got.Email != "ada@example.com" || got.DisplayName != "Ada" {
				t.Fatalf("identity = %+v", got)
			}
			if strings.Join(got.Groups, ",") != "eng,ops" || got.AssertionID != "id-assertion-1" {
				t.Fatalf("identity = %+v", got)
			}
			if !got.ReplayUntil.After(now.Add(5 * time.Minute)) {
				t.Fatalf("ReplayUntil %v is not past the assertion lifetime", got.ReplayUntil)
			}
		})
	}
}

func TestValidateRejects(t *testing.T) {
	idp := newTestIDP(t)
	sp := newTestSP(t, idp)
	forger := newTestIDP(t)
	now := time.Now()
	cases := []struct {
		name      string
		build     func(t *testing.T) string
		requestID string
	}{
		{"forged signature", func(t *testing.T) string { return forger.Respond(t, goodAssertion(idp, now), defaultOpts()) }, testRequestID},
		{"unsigned", func(t *testing.T) string {
			o := defaultOpts()
			o.SignAssert, o.SignResponse = false, false
			return idp.Respond(t, goodAssertion(idp, now), o)
		}, testRequestID},
		{"tampered after signing", func(t *testing.T) string {
			raw, _ := base64.StdEncoding.DecodeString(idp.Respond(t, goodAssertion(idp, now), defaultOpts()))
			return base64.StdEncoding.EncodeToString([]byte(strings.Replace(string(raw), "Ada@Example.com", "root@example.com", 1)))
		}, testRequestID},
		{"expired conditions", func(t *testing.T) string {
			a := goodAssertion(idp, now)
			a.Conditions.NotOnOrAfter = now.Add(-10 * time.Minute)
			return idp.Respond(t, a, defaultOpts())
		}, testRequestID},
		{"expired bearer confirmation", func(t *testing.T) string {
			a := goodAssertion(idp, now)
			a.Subject.SubjectConfirmations[0].SubjectConfirmationData.NotOnOrAfter = now.Add(-10 * time.Minute)
			return idp.Respond(t, a, defaultOpts())
		}, testRequestID},
		{"not yet valid", func(t *testing.T) string {
			a := goodAssertion(idp, now)
			a.Conditions.NotBefore = now.Add(time.Hour)
			return idp.Respond(t, a, defaultOpts())
		}, testRequestID},
		{"stale issue instant", func(t *testing.T) string {
			a := goodAssertion(idp, now.Add(-time.Hour))
			a.Conditions.NotOnOrAfter = now.Add(time.Hour)
			a.Subject.SubjectConfirmations[0].SubjectConfirmationData.NotOnOrAfter = now.Add(time.Hour)
			return idp.Respond(t, a, defaultOpts())
		}, testRequestID},
		{"missing NotOnOrAfter", func(t *testing.T) string {
			a := goodAssertion(idp, now)
			a.Conditions.NotOnOrAfter = time.Time{}
			return idp.Respond(t, a, defaultOpts())
		}, testRequestID},
		{"wrong audience", func(t *testing.T) string {
			a := goodAssertion(idp, now)
			a.Conditions.AudienceRestrictions[0].Audience.Value = "https://other-sp.example.test"
			return idp.Respond(t, a, defaultOpts())
		}, testRequestID},
		{"no audience restriction", func(t *testing.T) string {
			a := goodAssertion(idp, now)
			a.Conditions.AudienceRestrictions = nil
			return idp.Respond(t, a, defaultOpts())
		}, testRequestID},
		{"wrong recipient", func(t *testing.T) string {
			a := goodAssertion(idp, now)
			a.Subject.SubjectConfirmations[0].SubjectConfirmationData.Recipient = "https://evil.example.test/acs"
			return idp.Respond(t, a, defaultOpts())
		}, testRequestID},
		{"no bearer confirmation", func(t *testing.T) string {
			a := goodAssertion(idp, now)
			a.Subject.SubjectConfirmations = nil
			return idp.Respond(t, a, defaultOpts())
		}, testRequestID},
		{"wrong destination", func(t *testing.T) string {
			o := defaultOpts()
			o.Destination = "https://evil.example.test/acs"
			return idp.Respond(t, goodAssertion(idp, now), o)
		}, testRequestID},
		{"answers another request", func(t *testing.T) string { return idp.Respond(t, goodAssertion(idp, now), defaultOpts()) }, "id-other-request"},
		{"idp initiated", func(t *testing.T) string {
			a := goodAssertion(idp, now)
			a.Subject.SubjectConfirmations[0].SubjectConfirmationData.InResponseTo = ""
			o := defaultOpts()
			o.InResponseTo = ""
			return idp.Respond(t, a, o)
		}, ""},
		{"wrong issuer", func(t *testing.T) string {
			a := goodAssertion(idp, now)
			a.Issuer.Value = "https://evil-idp.example.test"
			o := defaultOpts()
			o.Issuer = "https://evil-idp.example.test"
			return idp.Respond(t, a, o)
		}, testRequestID},
		{"missing NameID", func(t *testing.T) string {
			a := goodAssertion(idp, now)
			a.Subject.NameID = nil
			return idp.Respond(t, a, defaultOpts())
		}, testRequestID},
		{"missing conditions", func(t *testing.T) string {
			a := goodAssertion(idp, now)
			a.Conditions = nil
			return idp.Respond(t, a, defaultOpts())
		}, testRequestID},
		{"failed status", func(t *testing.T) string {
			o := defaultOpts()
			o.Status = "urn:oasis:names:tc:SAML:2.0:status:Requester"
			return idp.Respond(t, goodAssertion(idp, now), o)
		}, testRequestID},
		{"not base64", func(*testing.T) string { return "%%%not-base64" }, testRequestID},
		{"empty", func(*testing.T) string { return "" }, testRequestID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := sp.Validate(tc.build(t), tc.requestID, Attributes{})
			if !errors.Is(err, ErrInvalidResponse) {
				t.Fatalf("Validate error = %v, want ErrInvalidResponse", err)
			}
		})
	}
}

func TestValidateIgnoresInjectedUnsignedAssertion(t *testing.T) {
	idp := newTestIDP(t)
	sp := newTestSP(t, idp)
	now := time.Now()
	o := defaultOpts()
	o.SignResponse = false
	raw, _ := base64.StdEncoding.DecodeString(idp.Respond(t, goodAssertion(idp, now), o))
	evil := goodAssertion(idp, now)
	evil.ID = "id-evil"
	setAttr(evil, "email", "root@example.com")
	evilXML := samltest.ElementString(t, evil)
	injected := strings.Replace(string(raw), "<saml:Assertion", evilXML+"<saml:Assertion", 1)
	got, err := sp.Validate(base64.StdEncoding.EncodeToString([]byte(injected)), testRequestID, Attributes{})
	if err == nil && (got.Email != "ada@example.com" || got.AssertionID != "id-assertion-1") {
		t.Fatalf("accepted the injected assertion: %+v", got)
	}
}

func TestValidateAttributeOverrides(t *testing.T) {
	idp := newTestIDP(t)
	sp := newTestSP(t, idp)
	now := time.Now()
	a := goodAssertion(idp, now)
	a.AttributeStatements[0].Attributes = append(a.AttributeStatements[0].Attributes,
		saml.Attribute{Name: "corpMail", Values: []saml.AttributeValue{{Value: "ada.l@corp.example"}}},
		saml.Attribute{Name: "roles", Values: []saml.AttributeValue{{Value: "admins"}}})
	got, err := sp.Validate(idp.Respond(t, a, defaultOpts()), testRequestID, Attributes{Email: "corpMail", Groups: "roles"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Email != "ada.l@corp.example" || strings.Join(got.Groups, ",") != "admins" {
		t.Fatalf("identity = %+v", got)
	}
}

func TestBeginLoginAndMetadata(t *testing.T) {
	sp := newTestSP(t, newTestIDP(t))
	redirect, id, err := sp.BeginLogin()
	if err != nil {
		t.Fatal(err)
	}
	if id == "" || !strings.HasPrefix(redirect, "https://idp.example.test/sso?") || !strings.Contains(redirect, "Signature=") {
		t.Fatalf("redirect %q id %q", redirect, id)
	}
	md, err := sp.MetadataXML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), testACS) || !strings.Contains(string(md), testSPEntity) {
		t.Fatalf("metadata missing acs or entity id:\n%s", md)
	}
	bad := sp
	bad.ACSURL = "http://cp.example.test/acs"
	if _, _, err := bad.BeginLogin(); err == nil {
		t.Fatal("BeginLogin accepted an http ACS URL")
	}
}
