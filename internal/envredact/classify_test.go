package envredact

import (
	"errors"
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name, key, value string
		override         *bool
		want             bool
		reason           string
	}{
		{"port stays plain", "PORT", "3000", nil, false, ""},
		{"node env stays plain", "NODE_ENV", "production", nil, false, ""},
		{"public url stays plain", "NEXT_PUBLIC_API_URL", "https://api.example.com", nil, false, ReasonPublic},
		{"public publishable key stays plain", "NEXT_PUBLIC_STRIPE_KEY", "pk_live_51HxYzAbCdEfGhIjKlMnOpQrStUvWx", nil, false, ReasonPublic},
		{"vite public token stays plain", "VITE_MAPBOX_TOKEN", "pk.eyJ1IjoiZXhhbXBsZSJ9.abcdefgh", nil, false, ReasonPublic},
		{"public flagged by operator", "NEXT_PUBLIC_STRIPE_KEY", "pk_live_x", &yes, true, ReasonOverride},
		{"api secret by name", "API_SECRET", "short", nil, true, ReasonName},
		{"api key by name", "API_KEY", "abc", nil, true, ReasonName},
		{"apikey joined", "STRIPEAPIKEY", "abc", nil, true, ReasonName},
		{"password", "DB_PASSWORD", "hunter2", nil, true, ReasonName},
		{"passwd", "SMTP_PASSWD", "x", nil, true, ReasonName},
		{"token", "GITHUB_TOKEN", "x", nil, true, ReasonName},
		{"private", "PRIVATE_KEY_PATH", "/k", nil, true, ReasonName},
		{"credential", "GOOGLE_APPLICATION_CREDENTIALS", "{}", nil, true, ReasonName},
		{"dsn", "SENTRY_DSN", "https://abc@o1.ingest.sentry.io/1", nil, true, ReasonName},
		{"database url", "DATABASE_URL", "postgres://localhost/app", nil, true, ReasonName},
		{"auth segment", "BASIC_AUTH", "x", nil, true, ReasonName},
		{"signing", "JWT_SIGNING_ALG", "HS256", nil, true, ReasonName},
		{"encryption key from report", "QUARTER_ENCRYPTION_KEY_V1", "ubahjctJ2K65pks5W9D7RDdT0ZYAax7hOtRPFO9Tfmc=", nil, true, ReasonName},
		{"oauth client id stays plain", "GITHUB_OAUTH_CLIENT_ID", "Iv1.8a61f9b3a7aba766", nil, false, ""},
		{"nextauth url stays plain", "NEXTAUTH_URL", "https://app.example.com", nil, false, ""},
		{"monkey is not a key", "MONKEY_MODE", "on", nil, false, ""},
		{"base64 value", "QUARTER_V1", "ubahjctJ2K65pks5W9D7RDdT0ZYAax7hOtRPFO9Tfmc=", nil, true, ReasonValue},
		{"hex value", "MASTER", "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", nil, true, ReasonValue},
		{"pem value", "TLS", "-----BEGIN PRIVATE KEY-----\nMIIE\n-----END PRIVATE KEY-----", nil, true, ReasonValue},
		{"jwt value", "BEARER", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl", nil, true, ReasonValue},
		{"url with password", "REDIS_URL", "redis://default:s3cr3t@redis:6379/0", nil, true, ReasonValue},
		{"url without password", "REDIS_URL", "redis://redis:6379/0", nil, false, ""},
		{"url with user only", "UPSTREAM", "https://user@example.com/x", nil, false, ""},
		{"known prefix", "STRIPE", "sk_live_abcdefghijklmnop1234", nil, true, ReasonValue},
		{"commit sha stays plain", "GIT_COMMIT_SHA", "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b", nil, false, ""},
		{"uuid stays plain", "TENANT", "123e4567-e89b-12d3-a456-426614174000", nil, false, ""},
		{"long lowercase word stays plain", "APP_NAME", "my-application-name-for-production", nil, false, ""},
		{"operator says plain", "API_SECRET", "x", &no, false, ReasonOverride},
		{"empty value", "FEATURE", "", nil, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reason := Classify(tt.key, tt.value, tt.override)
			if got != tt.want || reason != tt.reason {
				t.Fatalf("Classify(%q) = %v %q, want %v %q", tt.key, got, reason, tt.want, tt.reason)
			}
		})
	}
}

func TestPreview(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"short", ""},
		{"fifteen-chars-x", ""},
		{"sixteen-chars-xy", "sixt...s-xy"},
		{"ubahjctJ2K65pks5W9D7RDdT0ZYAax7hOtRPFO9Tfmc=", "ubah...fmc="},
	}
	for _, tt := range tests {
		if got := Preview(tt.in); got != tt.want {
			t.Errorf("Preview(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if got := Preview(tt.in); strings.Count(got, "") > 12 {
			t.Errorf("Preview(%q) too long: %q", tt.in, got)
		}
	}
}

func TestMapNeverLeaksSensitiveValues(t *testing.T) {
	env := map[string]string{
		"PORT":       "3000",
		"API_SECRET": "super-secret-value-123456",
		"QUARTER":    "ubahjctJ2K65pks5W9D7RDdT0ZYAax7hOtRPFO9Tfmc=",
		"MANUAL":     "plain-looking",
	}
	out, meta := Map(env, map[string]bool{"MANUAL": true})
	for _, k := range []string{"API_SECRET", "QUARTER", "MANUAL"} {
		if out[k] != Placeholder || !meta[k].ValueRedacted || !meta[k].Sensitive {
			t.Fatalf("%s not masked: %q %+v", k, out[k], meta[k])
		}
		if strings.Contains(meta[k].Preview, env[k]) && env[k] != "" {
			t.Fatalf("%s preview leaks value", k)
		}
	}
	if out["PORT"] != "3000" || meta["PORT"].Sensitive {
		t.Fatalf("PORT should stay plain: %q", out["PORT"])
	}
	if env["API_SECRET"] != "super-secret-value-123456" {
		t.Fatal("input map mutated")
	}
}

func TestMerge(t *testing.T) {
	stored := map[string]string{"API_SECRET": "real", "PORT": "3000"}
	got, err := Merge(map[string]string{"API_SECRET": Placeholder, "PORT": "4000", "NEW": "x"}, stored)
	if err != nil {
		t.Fatal(err)
	}
	if got["API_SECRET"] != "real" || got["PORT"] != "4000" || got["NEW"] != "x" {
		t.Fatalf("merge = %v", got)
	}
	if _, ok := got["missing"]; ok {
		t.Fatal("unexpected key")
	}
	_, err = Merge(map[string]string{"UNKNOWN": " " + Placeholder + " "}, stored)
	var pe *PlaceholderError
	if !errors.As(err, &pe) || pe.Key != "UNKNOWN" {
		t.Fatalf("want PlaceholderError for UNKNOWN, got %v", err)
	}
	if got, _ := Merge(nil, stored); got != nil {
		t.Fatal("nil submit should stay nil")
	}
}
