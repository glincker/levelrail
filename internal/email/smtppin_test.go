package email

import "testing"

func TestPinForOnlyMatchesItsAddress(t *testing.T) {
	sha := CertSHA256([]byte("leaf"))
	stored := FormatPin("10.0.0.5:587", sha)
	if got := PinFor(stored, "10.0.0.5:587"); got != sha {
		t.Fatalf("PinFor = %q", got)
	}
	for _, addr := range []string{"10.0.0.6:587", "10.0.0.5:465", ""} {
		if got := PinFor(stored, addr); got != "" {
			t.Fatalf("PinFor(%q) = %q, want empty", addr, got)
		}
	}
	if PinFor("10.0.0.5:587 short", "10.0.0.5:587") != "" {
		t.Fatal("a malformed pin must not apply")
	}
}
