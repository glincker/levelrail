package authengine

import "testing"

func TestFormatAAGUID(t *testing.T) {
	t.Parallel()
	raw := []byte{0xfb, 0xfc, 0x30, 0x07, 0x15, 0x4e, 0x4e, 0xcc, 0x8c, 0x0b, 0x6e, 0x02, 0x05, 0x57, 0xd7, 0xbd}
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"icloud", raw, "fbfc3007-154e-4ecc-8c0b-6e020557d7bd"},
		{"zero", make([]byte, 16), "00000000-0000-0000-0000-000000000000"},
		{"short", []byte{1, 2}, ""},
		{"nil", nil, ""},
	}
	for _, tc := range tests {
		if got := FormatAAGUID(tc.in); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestProviderForAAGUID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		id      string
		wantKey string
		name    string
	}{
		{"fbfc3007-154e-4ecc-8c0b-6e020557d7bd", ProviderApple, "iCloud Keychain"},
		{"FBFC3007-154E-4ECC-8C0B-6E020557D7BD", ProviderApple, "iCloud Keychain"},
		{"08987058-cadc-4b81-b6e1-30de50dcbe96", ProviderWindows, "Windows Hello"},
		{"bada5566-a7aa-401f-bd96-45619a55120d", ProviderOnePass, "1Password"},
		{"00000000-0000-0000-0000-000000000000", ProviderUnknown, ""},
		{"", ProviderUnknown, ""},
	}
	for _, tc := range tests {
		got := ProviderForAAGUID(tc.id)
		if got.Key != tc.wantKey || got.Name != tc.name {
			t.Errorf("%q: got %+v", tc.id, got)
		}
	}
}
