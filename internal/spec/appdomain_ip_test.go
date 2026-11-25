package spec

import "testing"

func TestValidateAppDomainRejectsIPLiterals(t *testing.T) {
	for _, d := range []string{"127.0.0.1", "10.0.0.5", "[::1]", "::1"} {
		if _, err := ValidateAppDomain(d); err == nil {
			t.Errorf("%q accepted, want rejection", d)
		}
	}
}
