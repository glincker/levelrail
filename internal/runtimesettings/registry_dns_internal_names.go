package runtimesettings

// Extra internal DNS names per app (beta, off by default).
const (
	KeyDNSInternalNamesEnabled  = "dns.internal_names_enabled"
	KeyDNSInternalNamesMax      = "dns.internal_names_max"
	KeyDNSInternalNameMaxLength = "dns.internal_name_max_length"
	KeyDNSInternalLabelMaxLen   = "dns.internal_label_max_length"
	KeyDNSInternalNamesReserved = "dns.internal_names_reserved"
)

func init() {
	Register(Definition{
		Key: KeyDNSInternalNamesEnabled, Category: CategoryObservability, Kind: KindBool, Default: "false",
		Env: "LEVELRAIL_DNS_INTERNAL_NAMES_ENABLED", Apply: ApplyLive,
		Title:       "Extra internal DNS names (beta)",
		Description: "Let an app claim extra names under the mesh zone that resolve to the same address as its own name. Off by default.",
	})
	Register(Definition{
		Key: KeyDNSInternalNamesMax, Category: CategoryObservability, Kind: KindInt, Default: "8",
		Env: "LEVELRAIL_DNS_INTERNAL_NAMES_MAX", Apply: ApplyLive, CheckValue: intRangeCheck(1, 100),
		Title:       "Most extra internal names per app",
		Description: "Upper bound on the dns names list of one app.",
	})
	Register(Definition{
		Key: KeyDNSInternalNameMaxLength, Category: CategoryObservability, Kind: KindInt, Default: "120",
		Env: "LEVELRAIL_DNS_INTERNAL_NAME_MAX_LENGTH", Apply: ApplyLive, CheckValue: intRangeCheck(1, 200),
		Title:       "Longest extra internal name",
		Description: "Most characters in one extra name, not counting the mesh zone.",
	})
	Register(Definition{
		Key: KeyDNSInternalLabelMaxLen, Category: CategoryObservability, Kind: KindInt, Default: "63",
		Env: "LEVELRAIL_DNS_INTERNAL_LABEL_MAX_LENGTH", Apply: ApplyLive, CheckValue: intRangeCheck(1, 63),
		Title:       "Longest extra internal name label",
		Description: "Most characters in one dot separated label of an extra name.",
	})
	Register(Definition{
		Key: KeyDNSInternalNamesReserved, Category: CategoryObservability, Kind: KindString,
		Default: "localhost,dns,ns,node,mesh,gateway,api",
		Env:     "LEVELRAIL_DNS_INTERNAL_NAMES_RESERVED", Apply: ApplyLive,
		Title:       "Reserved internal names",
		Description: "Comma separated names no app may claim as an extra internal name.",
	})
}
