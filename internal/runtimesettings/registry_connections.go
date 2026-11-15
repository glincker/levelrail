package runtimesettings

import "time"

// KeyConnectionExpiryWarn is how far ahead the integrations hub flags a connection as expiring.
const (
	KeyConnectionExpiryWarn = "connections.expiry_warn"
	connExpiryWarnLow       = time.Hour
	connExpiryWarnHigh      = 365 * 24 * time.Hour
)

// KeyConnectionBindingRulesMaxBytes caps the rules JSON a binding may store.
const KeyConnectionBindingRulesMaxBytes = "connections.binding_rules_max_bytes"

func init() {
	Register(Definition{
		Key: KeyConnectionExpiryWarn, Category: CategorySecurity, Kind: KindDuration, Default: "336h",
		Env: "APP_CONNECTION_EXPIRY_WARN", Apply: ApplyLive, CheckValue: durationRangeCheck(connExpiryWarnLow, connExpiryWarnHigh),
		Title:       "Connection expiry warning window",
		Description: "How long before a connection's credential expires the integrations page marks it as expiring, from 1h to 8760h.",
	})
	Register(Definition{
		Key: KeyConnectionBindingRulesMaxBytes, Category: CategorySecurity, Kind: KindInt, Default: "4096",
		Env: "APP_CONNECTION_BINDING_RULES_MAX_BYTES", Apply: ApplyLive, CheckValue: intRangeCheck(256, 65536),
		Title:       "Binding rules size limit",
		Description: "Largest rules document, in bytes, a connection binding may store, from 256 to 65536. Larger requests are refused. The next binding write uses a changed value.",
	})
}
