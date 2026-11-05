package runtimesettings

// KeyDNSVerifyTokenOnSave makes a DNS provider credential save also test it.
const KeyDNSVerifyTokenOnSave = "dns.verify_provider_on_save" //nolint:gosec // setting key, not a credential

func init() {
	Register(Definition{
		Key: KeyDNSVerifyTokenOnSave, Category: CategoryNetworking, Kind: KindBool, Default: "true",
		Env: "APP_DNS_VERIFY_PROVIDER_ON_SAVE", Apply: ApplyLive,
		Title:       "DNS: verify provider credentials on save",
		Description: "When a DNS provider token or key is saved, also list its zones with read-only calls and report which permissions are missing. The credential is saved either way and never shown. The next save uses a changed value.",
	})
}
