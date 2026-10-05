package netguard

import (
	"errors"
	"testing"
)

func TestControlAllowing(t *testing.T) {
	const ownEnv = "APP_TEST_ALLOW_PRIVATE_NETWORKS"
	tests := []struct {
		name    string
		own     string
		notify  string
		addr    string
		wantErr bool
	}{
		{"private blocked by default", "", "", "10.0.0.5:80", true},
		{"own override allows private", "true", "", "10.0.0.5:80", false},
		{"shared notify override still allows", "", "true", "10.0.0.5:80", false},
		{"public always passes", "", "", "93.184.216.34:443", false},
		{"metadata ip blocked by default", "", "", "169.254.169.254:80", true},
		{"false value does not allow", "false", "false", "127.0.0.1:80", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(ownEnv, tt.own)
			t.Setenv(AllowPrivateEnv, tt.notify)
			err := controlAllowing(ownEnv, AllowPrivateEnv)(tt.addr)
			if tt.wantErr != (err != nil) {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrBlockedAddress) {
				t.Errorf("err = %v, want ErrBlockedAddress", err)
			}
		})
	}
}
