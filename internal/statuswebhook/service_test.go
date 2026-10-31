package statuswebhook

import (
	"context"
	"strings"
	"testing"
)

func TestSignVerify(t *testing.T) {
	sig := Sign("whsec_x", "1700000000", []byte(`{"a":1}`))
	if !strings.HasPrefix(sig, "sha256=") || !Verify("whsec_x", "1700000000", []byte(`{"a":1}`), sig) {
		t.Fatal("signature does not round trip")
	}
	if Verify("whsec_x", "1700000001", []byte(`{"a":1}`), sig) || Verify("whsec_x", "1700000000", []byte(`{"a":2}`), sig) {
		t.Error("tampered timestamp or body verified")
	}
}

func TestValidateURLRefusesInternalTargets(t *testing.T) {
	s := New(nil, func() Sealer { return nil }, Config{}, nil)
	for _, u := range []string{"https://127.0.0.1/", "https://169.254.169.254/latest/meta-data", "https://[::1]/", "https://192.168.1.1/", "http://example.com/", "https://localhost/"} {
		if err := s.ValidateURL(context.Background(), u); err == nil {
			t.Errorf("%s accepted", u)
		}
	}
	if err := s.ValidateURL(context.Background(), "https://8.8.8.8/hook"); err != nil {
		t.Errorf("public https target refused: %v", err)
	}
}
