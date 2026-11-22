package runtimesettings

// DigitalOcean OAuth app settings. Both credentials empty keeps the Connect button hidden.
const (
	KeyCloudOAuthDOClientID     = "cloud.oauth.digitalocean.client_id"
	KeyCloudOAuthDOClientSecret = "cloud.oauth.digitalocean.client_secret" //nolint:gosec // setting key, not a credential
	KeyCloudOAuthDOTimeout      = "cloud.oauth.digitalocean.timeout_seconds"
	KeyCloudOAuthDORefreshLead  = "cloud.oauth.digitalocean.refresh_lead_seconds"
)

func init() {
	Register(Definition{
		Key: KeyCloudOAuthDOClientID, Category: CategorySecurity, Kind: KindString, Default: "",
		Env: "APP_CLOUD_OAUTH_DIGITALOCEAN_CLIENT_ID", Apply: ApplyLive,
		Title:       "DigitalOcean OAuth client id",
		Description: "Client id of the DigitalOcean OAuth app registered for this instance. With the client secret it turns on Connect DigitalOcean in the cloud account wizard. The redirect URI to register is the primary domain plus /api/v1/cloud-accounts/oauth/digitalocean/callback.",
	})
	Register(Definition{
		Key: KeyCloudOAuthDOClientSecret, Category: CategorySecurity, Kind: KindString, Default: "",
		Env: "APP_CLOUD_OAUTH_DIGITALOCEAN_CLIENT_SECRET", Apply: ApplyLive, Sensitive: true,
		Title:       "DigitalOcean OAuth client secret",
		Description: "Client secret of the DigitalOcean OAuth app. Used only to exchange and refresh tokens; never shown again once saved.",
	})
	Register(Definition{
		Key: KeyCloudOAuthDOTimeout, Category: CategorySecurity, Kind: KindInt, Default: "15",
		Env: "APP_CLOUD_OAUTH_DIGITALOCEAN_TIMEOUT_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(2, 120),
		Title:       "DigitalOcean OAuth: request timeout",
		Description: "Seconds a token exchange, refresh or revoke call to DigitalOcean may take, from 2 to 120. The next call uses a changed value.",
	})
	Register(Definition{
		Key: KeyCloudOAuthDORefreshLead, Category: CategorySecurity, Kind: KindInt, Default: "300",
		Env: "APP_CLOUD_OAUTH_DIGITALOCEAN_REFRESH_LEAD_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(0, 86400),
		Title:       "DigitalOcean OAuth: refresh lead time",
		Description: "Seconds before expiry that a connected DigitalOcean account refreshes its access token on use, from 0 to 86400. The next use applies a changed value.",
	})
}
