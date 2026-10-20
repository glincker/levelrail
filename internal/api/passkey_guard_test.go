package api

import "testing"

func TestRemovalLocksOut(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		passkeys    int
		hasPassword bool
		hasTOTP     bool
		want        bool
	}{
		{"last passkey, nothing else", 1, false, false, true},
		{"last passkey, has password", 1, true, false, false},
		{"last passkey, has 2fa", 1, false, true, false},
		{"second passkey remains", 2, false, false, false},
	}
	for _, tc := range tests {
		if got := removalLocksOut(tc.passkeys, tc.hasPassword, tc.hasTOTP); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestPasskeyResourceFromLibResolvesProvider(t *testing.T) {
	t.Parallel()
	synced := true
	got := passkeyResourceFromLib(authengineTestPasskey("fbfc3007-154e-4ecc-8c0b-6e020557d7bd", &synced))
	if got.ProviderKey != "apple" || got.ProviderName != "iCloud Keychain" || got.Synced == nil || !*got.Synced {
		t.Fatalf("unexpected resource: %+v", got)
	}
	unknown := passkeyResourceFromLib(authengineTestPasskey("", nil))
	if unknown.ProviderKey != "unknown" || unknown.ProviderName != "" || unknown.Synced != nil {
		t.Fatalf("unexpected unknown resource: %+v", unknown)
	}
}
