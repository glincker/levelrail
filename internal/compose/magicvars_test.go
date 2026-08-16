package compose

import "testing"

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
