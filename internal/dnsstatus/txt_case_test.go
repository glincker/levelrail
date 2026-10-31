package dnsstatus

import "testing"

func TestNormalizeTXTKeepsKeyCase(t *testing.T) {
	a := Normalize("TXT", "v=DKIM1; k=rsa; p=AbCd")
	b := Normalize("TXT", "v=dkim1; k=RSA; p=abcd")
	if a == b {
		t.Fatalf("p= key case was ignored: %q", a)
	}
	if Normalize("TXT", "v=DKIM1; p=AbCd") != Normalize("TXT", "V=dkim1; p=AbCd") {
		t.Fatal("tag case should not matter")
	}
}
