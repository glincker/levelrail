package erroringest

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

func TestTitleMasksRegisteredSecret(t *testing.T) {
	secret := "title-secret-value-5f3a9c"
	secretmask.Add(secret)
	ev, err := ParseStore([]byte(`{"exception":{"values":[{"type":"DialError","value":"auth ` + secret + ` rejected"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ev.Title, secret) || !strings.Contains(ev.Title, "rejected") {
		t.Fatalf("title = %q", ev.Title)
	}
}

func TestTitleSecretStraddlingCutIsMasked(t *testing.T) {
	secret := "straddle-secret-value-8d2e41"
	secretmask.Add(secret)
	val := strings.Repeat("a", maxTitle-len("E: ")-8) + secret
	ev, err := ParseStore([]byte(`{"exception":{"values":[{"type":"E","value":"` + val + `"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ev.Title, "straddle") || strings.Contains(ev.Title, "secret-v") {
		t.Fatalf("title leaks a secret prefix: %q", ev.Title)
	}
}
