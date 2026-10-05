package totp

import (
	"strings"
	"testing"
)

// TestGenerate_RFC4226KnownAnswers checks generate against the RFC 4226
// Appendix D test vectors for HOTP-SHA1, 6-digit truncation, secret
// ASCII "12345678901234567890". TOTP's counter is HOTP's counter
// derived from time instead of passed explicitly, so this is a direct
// correctness check of the shared HMAC-and-truncate step.
func TestGenerate_RFC4226KnownAnswers(t *testing.T) {
	key := []byte("12345678901234567890")
	want := []string{
		"755224", "287082", "359152", "969429", "338314",
		"254676", "287922", "162583", "399871", "520489",
	}
	for counter, code := range want {
		got := generate(key, int64(counter))
		if got != code {
			t.Errorf("generate(key, %d) = %q, want %q", counter, got, code)
		}
	}
}

func TestGenerateRecoveryCode(t *testing.T) {
	seen := make(map[string]bool)
	for range 20 {
		code, err := GenerateRecoveryCode()
		if err != nil {
			t.Fatalf("GenerateRecoveryCode: %v", err)
		}
		parts := strings.Split(code, "-")
		if len(parts) != 4 {
			t.Fatalf("GenerateRecoveryCode() = %q, want 4 hyphen-separated groups", code)
		}
		for _, p := range parts {
			if len(p) != 4 {
				t.Fatalf("GenerateRecoveryCode() = %q, want each group to be 4 characters", code)
			}
		}
		if seen[code] {
			t.Fatalf("GenerateRecoveryCode produced a duplicate: %q", code)
		}
		seen[code] = true
	}
}

func TestNormalizeRecoveryCode(t *testing.T) {
	got := NormalizeRecoveryCode(" ab12-cd34-ef56-gh78 ")
	want := "AB12CD34EF56GH78"
	if got != want {
		t.Errorf("NormalizeRecoveryCode = %q, want %q", got, want)
	}
}
