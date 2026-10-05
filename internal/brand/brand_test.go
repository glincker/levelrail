package brand

import (
	"os"
	"path/filepath"
	"testing"
)

func writeYAML(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "brand.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestLoad(t *testing.T) {
	const valid = `
name: Levelrail
short_name: Levelrail
binary_name: levelrail
domain: glinr.com/levelrail
support_url: https://github.com/GLINCKER/levelrail/issues
support_email: support@levelrail.com
docs_url: https://glinr.com/levelrail/docs
`

	tests := []struct {
		name    string
		yaml    string
		env     map[string]string
		wantErr bool
		check   func(t *testing.T, b *Brand)
	}{
		{
			name: "valid file loads all fields",
			yaml: valid,
			check: func(t *testing.T, b *Brand) {
				if b.Name != "Levelrail" {
					t.Errorf("Name = %q, want Levelrail", b.Name)
				}
				if b.BinaryName != "levelrail" {
					t.Errorf("BinaryName = %q, want levelrail", b.BinaryName)
				}
			},
		},
		{
			name: "env override wins over file",
			yaml: valid,
			env:  map[string]string{"APP_BRAND_NAME": "Override"},
			check: func(t *testing.T, b *Brand) {
				if b.Name != "Override" {
					t.Errorf("Name = %q, want Override", b.Name)
				}
			},
		},
		{
			name: "support email loads from file and can be overridden",
			yaml: valid,
			check: func(t *testing.T, b *Brand) {
				if b.SupportEmail != "support@levelrail.com" {
					t.Errorf("SupportEmail = %q, want support@levelrail.com", b.SupportEmail)
				}
			},
		},
		{
			name: "support email env override wins over file",
			yaml: valid,
			env:  map[string]string{"APP_BRAND_SUPPORT_EMAIL": "support@glincker.com"},
			check: func(t *testing.T, b *Brand) {
				if b.SupportEmail != "support@glincker.com" {
					t.Errorf("SupportEmail = %q, want support@glincker.com", b.SupportEmail)
				}
			},
		},
		{
			name: "primary_color_dark falls back to primary_color when empty",
			yaml: valid + "\nprimary_color: \"#107292\"\n",
			check: func(t *testing.T, b *Brand) {
				if b.PrimaryColorDark != "#107292" {
					t.Errorf("PrimaryColorDark = %q, want #107292", b.PrimaryColorDark)
				}
			},
		},
		{
			name: "primary_color_dark loads from file when set",
			yaml: valid + "\nprimary_color: \"#107292\"\nprimary_color_dark: \"#2fb3dc\"\n",
			check: func(t *testing.T, b *Brand) {
				if b.PrimaryColorDark != "#2fb3dc" {
					t.Errorf("PrimaryColorDark = %q, want #2fb3dc", b.PrimaryColorDark)
				}
			},
		},
		{
			name: "primary_color_dark env override wins over file",
			yaml: valid + "\nprimary_color: \"#107292\"\nprimary_color_dark: \"#2fb3dc\"\n",
			env:  map[string]string{"APP_BRAND_PRIMARY_COLOR_DARK": "#ff0000"},
			check: func(t *testing.T, b *Brand) {
				if b.PrimaryColorDark != "#ff0000" {
					t.Errorf("PrimaryColorDark = %q, want #ff0000", b.PrimaryColorDark)
				}
			},
		},
		{
			name:    "missing name fails validation",
			yaml:    `binary_name: levelrail`,
			wantErr: true,
		},
		{
			name:    "missing binary_name fails validation",
			yaml:    `name: Levelrail`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			path := writeYAML(t, tt.yaml)

			b, err := Load(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.check != nil {
				tt.check(t, b)
			}
		})
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestRuleCommentPrefixMatchesLegacyDefault(t *testing.T) {
	b, err := Load("../../brand.yaml")
	if err != nil {
		t.Fatalf("load default brand: %v", err)
	}
	if got, want := b.RuleCommentPrefix(), "level"+"rail:"; got != want {
		t.Fatalf("RuleCommentPrefix() = %q, want %q", got, want)
	}
}
