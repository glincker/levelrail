package totp

import (
	"strings"
	"testing"
	"time"
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

func TestGenerateSecret_RoundTripsThroughValidate(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	if secret == "" {
		t.Fatal("GenerateSecret returned an empty secret")
	}

	key, err := decodeSecret(secret)
	if err != nil {
		t.Fatalf("decodeSecret(%q): %v", secret, err)
	}

	now := time.Unix(1_700_000_000, 0)
	code := generate(key, now.Unix()/stepSeconds)
	if !Validate(secret, code, now) {
		t.Fatalf("Validate(secret, %q, now) = false, want true", code)
	}
	if Validate(secret, "000001", now) && code != "000001" {
		t.Fatal("Validate accepted an arbitrary wrong code")
	}
}

func TestValidate_ToleratesOneStepClockSkew(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	key, err := decodeSecret(secret)
	if err != nil {
		t.Fatalf("decodeSecret: %v", err)
	}

	now := time.Unix(1_700_000_000, 0)
	counter := now.Unix() / stepSeconds

	oneStepBack := generate(key, counter-1)
	if !Validate(secret, oneStepBack, now) {
		t.Error("Validate rejected a code from one step in the past, want acceptance within skew")
	}

	twoStepsBack := generate(key, counter-2)
	if Validate(secret, twoStepsBack, now) {
		t.Error("Validate accepted a code from two steps in the past, want rejection outside skew")
	}
}

func TestValidate_RejectsMalformedCode(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	now := time.Unix(1_700_000_000, 0)

	cases := []string{"", "12345", "1234567", "abcdef", "12 345"}
	for _, code := range cases {
		if Validate(secret, code, now) {
			t.Errorf("Validate(secret, %q, now) = true, want false", code)
		}
	}
}

func TestProvisioningURI(t *testing.T) {
	uri := ProvisioningURI("JBSWY3DPEHPK3PXP", "admin@example.com", "Levelrail (deploy.example.com)")
	if !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Fatalf("ProvisioningURI = %q, want otpauth://totp/ prefix", uri)
	}
	if !strings.Contains(uri, "secret=JBSWY3DPEHPK3PXP") {
		t.Errorf("ProvisioningURI %q missing secret param", uri)
	}
	if !strings.Contains(uri, "period=30") || !strings.Contains(uri, "digits=6") {
		t.Errorf("ProvisioningURI %q missing expected period/digits params", uri)
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
