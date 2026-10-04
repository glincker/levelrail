package integrations

import (
	"strings"
	"testing"
)

func TestCatalogEntriesAreWellFormed(t *testing.T) {
	validTypes := map[string]bool{
		FieldTypeAPIKey:    true,
		FieldTypeDSN:       true,
		FieldTypeToken:     true,
		FieldTypeProjectID: true,
		FieldTypeSite:      true,
		FieldTypeHost:      true,
	}

	seenKeys := map[string]bool{}
	for _, entry := range Catalog {
		t.Run(entry.Key, func(t *testing.T) {
			if entry.Key == "" {
				t.Error("key is empty")
			}
			if seenKeys[entry.Key] {
				t.Errorf("duplicate key %q", entry.Key)
			}
			seenKeys[entry.Key] = true

			if entry.Name == "" {
				t.Error("name is empty")
			}
			if entry.Description == "" {
				t.Error("description is empty")
			}
			if !strings.HasPrefix(entry.DocsURL, "https://") {
				t.Errorf("docs url %q does not look like a valid https url", entry.DocsURL)
			}
			if len(entry.EnvVars) == 0 {
				t.Error("entry has no env vars")
			}
			for _, ev := range entry.EnvVars {
				if ev.Name == "" {
					t.Error("env var has empty name")
				}
				if !validTypes[ev.Type] {
					t.Errorf("env var %q has unknown type %q", ev.Name, ev.Type)
				}
				if !ev.Required && ev.Default == "" && ev.Placeholder == "" {
					t.Errorf("optional env var %q has neither a default nor a placeholder", ev.Name)
				}
			}
		})
	}
}

func TestGetKnownEntries(t *testing.T) {
	for _, key := range []string{
		"sentry", "posthog", "datadog", "axiom", "betterstack",
		"logsnag", "bugsnag", "newrelic", "glitchtip", "oneuptime", "plausible",
	} {
		if _, ok := Get(key); !ok {
			t.Errorf("Get(%q) = not found, want a catalog entry", key)
		}
	}
}

func TestGetUnknownKey(t *testing.T) {
	if _, ok := Get("does-not-exist"); ok {
		t.Error("Get(unknown) = found, want not found")
	}
}

func TestGlitchTipSharesSentryDSN(t *testing.T) {
	glitchtip, ok := Get("glitchtip")
	if !ok {
		t.Fatal("glitchtip entry missing")
	}
	if len(glitchtip.EnvVars) == 0 || glitchtip.EnvVars[0].Name != "SENTRY_DSN" {
		t.Errorf("glitchtip env vars = %+v, want SENTRY_DSN first (Sentry-SDK-compatible)", glitchtip.EnvVars)
	}
}
