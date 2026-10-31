package dnsstatus

import (
	"strings"
	"testing"
)

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

func TestClassifyTwoSPFRecordsIsConflict(t *testing.T) {
	w := Want{ID: "spf", Type: "TXT", Name: "example.com", Value: "v=spf1 include:mail.example.net ~all"}
	one := classify(w, []Found{{Type: "TXT", Name: "example.com", Value: w.Value}})
	if one.Status != Live {
		t.Fatalf("single matching SPF should be live, got %s", one.Status)
	}
	two := classify(w, []Found{
		{Type: "TXT", Name: "example.com", Value: w.Value},
		{Type: "TXT", Name: "example.com", Value: "v=spf1 include:other.example ~all"},
	})
	if two.Status != Conflict || !strings.HasPrefix(two.Reason, "spf_multiple") {
		t.Fatalf("two SPF records must conflict with spf_multiple, got %s %q", two.Status, two.Reason)
	}
}
