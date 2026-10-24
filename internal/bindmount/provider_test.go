package bindmount

import "testing"

func TestValidateHostPath_Provider(t *testing.T) {
	tests := []struct {
		name     string
		settings Settings
		hostPath string
		wantErr  bool
	}{
		{"credential dir blocked", Settings{BlockCredentialDirs: true}, "/srv/u/.ssh/keys", true},
		{"credential dir allowed when toggle off", Settings{BlockCredentialDirs: false}, "/srv/u/.ssh/keys", false},
		{"inside allowed root", Settings{BlockCredentialDirs: true, AllowedRoots: []string{"/srv/apps"}}, "/srv/apps/a", false},
		{"outside allowed root", Settings{BlockCredentialDirs: true, AllowedRoots: []string{"/srv/apps"}}, "/srv/other", true},
		{"forbidden path stays blocked with toggle off", Settings{BlockCredentialDirs: false}, "/etc/x", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetProvider(func() Settings { return tt.settings })
			t.Cleanup(func() { SetProvider(nil) })
			if err := ValidateHostPath(tt.hostPath); (err != nil) != tt.wantErr {
				t.Errorf("ValidateHostPath(%q) = %v, wantErr %v", tt.hostPath, err, tt.wantErr)
			}
		})
	}
}

func TestValidateHostPath_EnvFallback(t *testing.T) {
	SetProvider(nil)
	t.Setenv(EnvBlockCredentialDirs, "0")
	if err := ValidateHostPath("/srv/u/.aws/x"); err != nil {
		t.Errorf("env toggle off should allow credential dirs: %v", err)
	}
	t.Setenv(EnvBlockCredentialDirs, "")
	if err := ValidateHostPath("/srv/u/.aws/x"); err == nil {
		t.Error("credential dirs must be blocked by default")
	}
}
