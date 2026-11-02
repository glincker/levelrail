package compose

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestFindMagicVars(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  MagicVar
	}{
		{
			name:  "braced with length hint",
			value: "${SERVICE_PASSWORD_64_APIKEY}",
			want:  MagicVar{Token: "${SERVICE_PASSWORD_64_APIKEY}", Kind: "PASSWORD", Key: "APIKEY", Length: 64, Generatable: true},
		},
		{
			name:  "bare, no length hint",
			value: "$SERVICE_PASSWORD_ENCRYPTIONKEY",
			want:  MagicVar{Token: "$SERVICE_PASSWORD_ENCRYPTIONKEY", Kind: "PASSWORD", Key: "ENCRYPTIONKEY", Generatable: true},
		},
		{
			name:  "fqdn is not generatable",
			value: "${SERVICE_FQDN_ACTIVEPIECES}",
			want:  MagicVar{Token: "${SERVICE_FQDN_ACTIVEPIECES}", Kind: "FQDN", Key: "ACTIVEPIECES"}, //nolint:gosec // MagicVar.Key is a var name, not a credential value
		},
		{
			name:  "braced with default",
			value: "${SERVICE_DB_NAME:-ente_db}",
			want:  MagicVar{Token: "${SERVICE_DB_NAME:-ente_db}", Kind: "DB", Key: "NAME", Default: "ente_db", HasDefault: true}, //nolint:gosec // MagicVar.Key is a var name, not a credential value
		},
		{
			name:  "braced with empty default",
			value: "${SERVICE_ROLE_KEY_ASYMMETRIC:-}",
			want:  MagicVar{Token: "${SERVICE_ROLE_KEY_ASYMMETRIC:-}", Kind: "ROLE", Key: "KEY_ASYMMETRIC", Default: "", HasDefault: true}, //nolint:gosec // MagicVar.Key is a var name, not a credential value
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindMagicVars(tt.value)
			if len(got) != 1 {
				t.Fatalf("FindMagicVars(%q) returned %d vars, want 1", tt.value, len(got))
			}
			if got[0] != tt.want {
				t.Errorf("FindMagicVars(%q) = %+v, want %+v", tt.value, got[0], tt.want)
			}
		})
	}
}

func TestFindMagicVars_NoMatch(t *testing.T) {
	got := FindMagicVars("production")
	if len(got) != 0 {
		t.Errorf("FindMagicVars() = %+v, want empty", got)
	}
}

func TestFindMagicVars_Multiple(t *testing.T) {
	got := FindMagicVars("postgres://$SERVICE_USER_DB:$SERVICE_PASSWORD_DB@db/app")
	if len(got) != 2 {
		t.Fatalf("got %d vars, want 2", len(got))
	}
	if got[0].Kind != "USER" || got[1].Kind != "PASSWORD" {
		t.Errorf("got kinds %q, %q, want USER, PASSWORD", got[0].Kind, got[1].Kind)
	}
}

func TestGenerateValue(t *testing.T) {
	tests := []struct {
		kind       string
		wantLength int
	}{
		{"PASSWORD", defaultPasswordLength},
		{"USER", defaultUsernameLength},
		{"HEX", defaultHexLength},
		{"BASE64", defaultBase64Length},
		{"REALBASE64", defaultBase64Length},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			got, err := GenerateValue(tt.kind, 0)
			if err != nil {
				t.Fatalf("GenerateValue(%q, 0) error = %v", tt.kind, err)
			}
			if len(got) != tt.wantLength {
				t.Errorf("GenerateValue(%q, 0) length = %d, want %d", tt.kind, len(got), tt.wantLength)
			}
		})
	}
}

func TestGenerateValue_ExplicitLength(t *testing.T) {
	got, err := GenerateValue("HEX", 10)
	if err != nil {
		t.Fatalf("GenerateValue() error = %v", err)
	}
	if len(got) != 10 {
		t.Errorf("len = %d, want 10", len(got))
	}
}

func TestGenerateValue_TwoCallsDiffer(t *testing.T) {
	a, err := GenerateValue("PASSWORD", 0)
	if err != nil {
		t.Fatalf("GenerateValue() error = %v", err)
	}
	b, err := GenerateValue("PASSWORD", 0)
	if err != nil {
		t.Fatalf("GenerateValue() error = %v", err)
	}
	if a == b {
		t.Error("two consecutive GenerateValue calls produced the same value, want randomness")
	}
}

func TestGenerateValue_UnknownKind(t *testing.T) {
	if _, err := GenerateValue("FQDN", 0); err == nil {
		t.Fatal("GenerateValue(\"FQDN\", 0) error = nil, want an error (not a generatable kind)")
	}
}

func TestBase64BytesResolvesInsideComposedValue(t *testing.T) {
	f := &File{Services: map[string]Service{"app": {Environment: Environment{
		"APP_KEY": "base64:$SERVICE_BASE64BYTES_32_APPKEY",
	}}}}
	persisted := map[string]string{}
	_, unresolved, err := ResolveMagicVars(f, func(kind, _ string, n int) (string, error) {
		return GenerateValue(kind, n)
	}, func(_, envKey, v string) error { persisted[envKey] = v; return nil })
	if err != nil || len(unresolved) != 0 {
		t.Fatalf("err=%v unresolved=%v", err, unresolved)
	}
	val, ok := strings.CutPrefix(persisted["APP_KEY"], "base64:")
	if !ok {
		t.Fatalf("missing prefix: %q", persisted["APP_KEY"])
	}
	raw, err := base64.StdEncoding.DecodeString(val)
	if err != nil || len(raw) != 32 {
		t.Fatalf("decode err=%v len=%d, want 32 bytes", err, len(raw))
	}
}

func TestTranslateMagicVars(t *testing.T) {
	src := "services:\n  app:\n    environment:\n      - SERVICE_FQDN_APP_3000\n      - SERVICE_URL_APP\n      - DB_PASS=${SERVICE_PASSWORD_DB}\n      - SERVICE_PASSWORD_DB\n      - X=${SERVICE_NAME_THING}\n      - Y=${SERVICE_OTHER_Z:-fallback}\n"
	got, err := TranslateMagicVars(src, func(k string) string { return strings.ToLower(k) + ".example.test" })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SERVICE_FQDN_APP_3000=app.example.test", "SERVICE_URL_APP=https://app.example.test", "Y=fallback", "${SERVICE_NAME_THING}"} {
		if !strings.Contains(got.Compose, want) {
			t.Errorf("missing %q in:\n%s", want, got.Compose)
		}
	}
	if len(got.Unsupported) != 1 || got.Unsupported[0] != "SERVICE_NAME_THING" {
		t.Errorf("Unsupported = %v", got.Unsupported)
	}
	if len(got.Generated) != 1 {
		t.Fatalf("Generated = %v, want one shared password", got.Generated)
	}
	var vals []string
	for _, l := range strings.Split(got.Compose, "\n") {
		if strings.Contains(l, "DB_PASS=") || strings.Contains(l, "SERVICE_PASSWORD_DB=") {
			vals = append(vals, l[strings.LastIndex(l, "=")+1:])
		}
	}
	if len(vals) != 2 || vals[0] != vals[1] || len(vals[0]) != 32 {
		t.Errorf("password values = %v, want identical 32 char values", vals)
	}
}
