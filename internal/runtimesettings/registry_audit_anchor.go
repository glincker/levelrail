package runtimesettings

// Audit anchor webhook export keys. The URL empty means the export is off.
const (
	KeyAuditAnchorWebhookURL    = "audit.anchor_webhook_url"
	KeyAuditAnchorWebhookSecret = "audit.anchor_webhook_secret" //nolint:gosec // setting key, not a credential
)

func init() {
	Register(Definition{
		Key: KeyAuditAnchorWebhookURL, Category: CategorySecurity, Kind: KindString, Default: "",
		Env: "APP_AUDIT_ANCHOR_WEBHOOK_URL", Apply: ApplyLive, Risk: RiskWeakensSecurity,
		Title:       "Audit anchor webhook URL",
		Description: "HTTPS endpoint that receives each new signed audit checkpoint (sequence, hash, signature, timestamp, never log bodies). Empty turns the export off.",
	})
	Register(Definition{
		Key: KeyAuditAnchorWebhookSecret, Category: CategorySecurity, Kind: KindString, Default: "",
		Env: "APP_AUDIT_ANCHOR_WEBHOOK_SECRET", Apply: ApplyLive, Sensitive: true,
		Title:       "Audit anchor webhook secret",
		Description: "Shared secret used to HMAC-SHA256 sign each export body, sent as X-Levelrail-Signature. Empty sends the export unsigned.",
	})
}
