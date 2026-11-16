package authengine

import (
	"testing"

	"github.com/crewjam/saml"
)

func TestHasNameIDRejectsMissingSubjectIdentity(t *testing.T) {
	cases := map[string]*saml.Assertion{
		"nil assertion": nil,
		"no subject":    {},
		"nil nameid":    {Subject: &saml.Subject{}},
		"empty nameid":  {Subject: &saml.Subject{NameID: &saml.NameID{Value: "  "}}},
	}
	for name, a := range cases {
		if hasNameID(a) {
			t.Errorf("%s: want rejected", name)
		}
	}
	ok := &saml.Assertion{Subject: &saml.Subject{NameID: &saml.NameID{Value: "u-1"}}}
	if !hasNameID(ok) {
		t.Error("assertion with a NameID must pass")
	}
}
