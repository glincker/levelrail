package runtimesettings

// KeyAuthSSOAutoRedirect sends the sign-in page straight to the only enabled single sign-on provider.
const KeyAuthSSOAutoRedirect = "auth.sso_auto_redirect"

func init() {
	Register(Definition{
		Key: KeyAuthSSOAutoRedirect, Category: CategorySecurity, Kind: KindBool, Default: "false",
		Apply:       ApplyLive,
		Title:       "Redirect to single sign-on automatically",
		Description: "When exactly one single sign-on provider is enabled, the sign-in page goes straight to it. Open the sign-in page with ?password=1 to use the password form instead, so a broken provider can never lock admins out. Off by default.",
	})
}
