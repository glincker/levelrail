package identity

import (
	"errors"
	"strings"
	"testing"
)

// u builds a string from code points so the source holds no bidi or invisible characters.
func u(parts ...any) string {
	var b strings.Builder
	for _, p := range parts {
		switch v := p.(type) {
		case string:
			b.WriteString(v)
		case rune:
			b.WriteRune(v)
		}
	}
	return b.String()
}

func TestNormalizeEmail(t *testing.T) {
	cases := []struct{ in, want string }{
		{"  Alice@Example.COM ", "alice@example.com"},
		{u("e", rune(0x301), "@x.io"), u(rune(0xE9), "@x.io")},
		{u(rune(0xFF21), "lice@x.io"), "alice@x.io"},
		{u(rune(0x212A), "elvin@x.io"), "kelvin@x.io"},
	}
	for _, c := range cases {
		if got := NormalizeEmail(c.in); got != c.want {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidateEmail(t *testing.T) {
	bad := []string{
		"", u("a", rune(0x202E), "b@x.io"), u("a", rune(0x200B), "b@x.io"), u("a", rune(0x2066), "b@x.io"),
		"a\x00b@x.io", "a b@x.io", "a\nb@x.io", u("a", rune(0x3164), "b@x.io"), "\xff@x.io",
		strings.Repeat("a", 250) + "@x.io",
	}
	for _, s := range bad {
		if _, err := ValidateEmail(s); !errors.Is(err, ErrInvalid) {
			t.Errorf("ValidateEmail(%q) = %v, want ErrInvalid", s, err)
		}
	}
	for _, s := range []string{"a@x.io", "admin", u(rune(0xFC), "ser@x.de")} {
		if _, err := ValidateEmail(s); err != nil {
			t.Errorf("ValidateEmail(%q) = %v", s, err)
		}
	}
}

func TestValidateName(t *testing.T) {
	bad := []string{
		"", "   ", u("a", rune(0x202E), "b"), u("a", rune(0x200B), "b"), "a\tb", "a\nb", u("a", rune(0x2060), "b"),
		u("x", rune(0x2028), "y"), "a\x00b", u("a", rune(0xE000), "b"),
		strings.Repeat("x", MaxNameRunes+1), strings.Repeat(u(rune(0x1F600)), 101),
	}
	for _, s := range bad {
		if _, err := ValidateName("name", s); !errors.Is(err, ErrInvalid) {
			t.Errorf("ValidateName(%q) = %v, want ErrInvalid", s, err)
		}
	}
	got, err := ValidateName("name", u(" Cafe", rune(0x301), " ", rune(0x645), rune(0x6CC), rune(0x200C), rune(0x62E), " "))
	if err != nil || !strings.HasPrefix(got, u("Caf", rune(0xE9))) {
		t.Errorf("valid name = %q, %v", got, err)
	}
}

func TestSanitize(t *testing.T) {
	if got := Sanitize(u("a", rune(0x202E), "b\nc\x00d")); got != u("a", rune(0xFFFD), "b", rune(0xFFFD), "c", rune(0xFFFD), "d") {
		t.Errorf("Sanitize = %q", got)
	}
	if got := Sanitize("plain name"); got != "plain name" {
		t.Errorf("Sanitize changed plain text: %q", got)
	}
}
