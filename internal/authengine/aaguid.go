package authengine

import "strings"

// Provider keys are stable identifiers the dashboard maps to an icon.
const (
	ProviderUnknown   = "unknown"
	ProviderApple     = "apple"
	ProviderGoogle    = "google"
	ProviderWindows   = "windows"
	ProviderChrome    = "chrome"
	ProviderEdge      = "edge"
	ProviderSamsung   = "samsung"
	ProviderOnePass   = "1password"
	ProviderBitwarden = "bitwarden"
	ProviderDashlane  = "dashlane"
	ProviderKeeper    = "keeper"
	ProviderNordPass  = "nordpass"
	ProviderEnpass    = "enpass"
	ProviderYubiKey   = "yubikey"
)

// PasskeyProvider names the authenticator that stored a passkey.
type PasskeyProvider struct {
	Key  string
	Name string
}

var aaguidProviders = map[string]PasskeyProvider{
	"fbfc3007-154e-4ecc-8c0b-6e020557d7bd": {ProviderApple, "iCloud Keychain"},
	"dd4ec289-e01d-41c9-bb89-70fa845d4bf2": {ProviderApple, "iCloud Keychain (Managed)"},
	"ea9b8d66-4d01-1d21-3ce4-b6b48cb575d4": {ProviderGoogle, "Google Password Manager"},
	"adce0002-35bc-c60a-648b-0b25f1f05503": {ProviderChrome, "Chrome on Mac"},
	"b5397666-4885-aa6b-cebf-e52262a439a2": {ProviderChrome, "Chromium Browser"},
	"771b48fd-d3d4-4f74-9232-fc157ab0507a": {ProviderEdge, "Edge on Mac"},
	"08987058-cadc-4b81-b6e1-30de50dcbe96": {ProviderWindows, "Windows Hello"},
	"9ddd1817-af5a-4672-a2b9-3e3dd95000a9": {ProviderWindows, "Windows Hello"},
	"6028b017-b1d4-4c02-b4b3-afcdafc96bb2": {ProviderWindows, "Windows Hello"},
	"53414d53-554e-4700-0000-000000000000": {ProviderSamsung, "Samsung Pass"},
	"bada5566-a7aa-401f-bd96-45619a55120d": {ProviderOnePass, "1Password"},
	"d548826e-79b4-db40-a3d8-11116f7e8349": {ProviderBitwarden, "Bitwarden"},
	"531126d6-e717-415c-9320-3d9aa6981239": {ProviderDashlane, "Dashlane"},
	"0ea242b4-43c4-4a1b-8b17-dd6d0b6baec6": {ProviderKeeper, "Keeper"},
	"b84e4048-15dc-4dd0-8640-f4f60813c8af": {ProviderNordPass, "NordPass"},
	"f3809540-7f14-49c1-a8b3-8f813b225541": {ProviderEnpass, "Enpass"},
	"fa2b99dc-9e39-4257-8f92-4a30d23c4118": {ProviderYubiKey, "YubiKey 5 Series"},
	"2fc0579f-8113-47ea-b116-bb5a8db9202a": {ProviderYubiKey, "YubiKey 5 NFC"},
	"cb69481e-8ff7-4039-93ec-0a2729a154a8": {ProviderYubiKey, "YubiKey 5 Series"},
	"ee882879-721c-4913-9775-3dfcce97072a": {ProviderYubiKey, "YubiKey 5 Series"},
	"73bb0cd4-e502-49b8-9c6f-b59445bf720b": {ProviderYubiKey, "YubiKey 5 FIPS"},
	"6d44ba9b-f6ec-2e49-b930-0c8fe920cb73": {ProviderYubiKey, "Security Key by Yubico"},
}

// FormatAAGUID renders 16 raw bytes as the canonical lowercase UUID, or "" for any other length.
func FormatAAGUID(b []byte) string {
	if len(b) != 16 {
		return ""
	}
	const hexdigits = "0123456789abcdef"
	var sb strings.Builder
	for i, v := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			sb.WriteByte('-')
		}
		sb.WriteByte(hexdigits[v>>4])
		sb.WriteByte(hexdigits[v&0x0f])
	}
	return sb.String()
}

// ProviderForAAGUID resolves an authenticator id to its provider; unknown ids return ProviderUnknown with an empty name.
func ProviderForAAGUID(aaguid string) PasskeyProvider {
	if p, ok := aaguidProviders[strings.ToLower(aaguid)]; ok {
		return p
	}
	return PasskeyProvider{Key: ProviderUnknown}
}
