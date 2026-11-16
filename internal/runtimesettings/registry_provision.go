package runtimesettings

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Cloud server provisioning guards. Env values only seed the defaults.
const (
	KeyProvisionMaxServers        = "provision.max_servers"
	KeyProvisionMaxMonthlyPrice   = "provision.max_monthly_price_usd"
	KeyProvisionAllowUnknownPrice = "provision.allow_unknown_price"
	KeyProvisionEURToUSDRate      = "provision.eur_to_usd_rate"
	KeyProvisionEnrollTimeout     = "provision.enroll_timeout"
	provisionMaxServersCeil       = 1000
)

// decimalCheck accepts a non-negative decimal, or empty when allowEmpty.
func decimalCheck(allowEmpty bool) func(string) error {
	return func(v string) error {
		v = strings.TrimSpace(v)
		if v == "" && allowEmpty {
			return nil
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 || f > 1e6 {
			return fmt.Errorf("value must be a decimal number from 0 to 1000000")
		}
		return nil
	}
}

func init() {
	cat := CategoryExperimental
	registerLimit(Definition{
		Key: KeyProvisionMaxServers, Category: cat, Default: "1", Env: "APP_PROVISION_MAX_SERVERS", Apply: ApplyLive,
		Title:       "Cloud servers: most at once",
		Description: "Most provisioned cloud servers that may exist at once. New provisions beyond this are refused before any provider call.",
	}, 1, provisionMaxServersCeil)
	Register(Definition{
		Key: KeyProvisionMaxMonthlyPrice, Category: cat, Kind: KindString, Default: "25", Env: "APP_PROVISION_MAX_MONTHLY_PRICE_USD", Apply: ApplyLive,
		CheckValue:  decimalCheck(true),
		Title:       "Cloud servers: monthly price cap (USD)",
		Description: "Refuses a provision whose listed monthly price is above this many US dollars. Empty means no cap.",
	})
	Register(Definition{
		Key: KeyProvisionAllowUnknownPrice, Category: cat, Kind: KindBool, Default: "false", Env: "APP_PROVISION_ALLOW_UNKNOWN_PRICE", Apply: ApplyLive,
		Risk:        RiskWeakensSecurity,
		Title:       "Cloud servers: allow unpriced sizes",
		Description: "Lets a provision proceed when the provider lists no price or an unsupported currency, which skips the price cap.",
	})
	Register(Definition{
		Key: KeyProvisionEURToUSDRate, Category: cat, Kind: KindString, Default: "1.10", Env: "APP_PROVISION_EUR_TO_USD_RATE", Apply: ApplyLive,
		CheckValue:  decimalCheck(false),
		Title:       "Cloud servers: EUR to USD rate",
		Description: "Rate used to compare EUR-priced sizes against the USD price cap.",
	})
	Register(Definition{
		Key: KeyProvisionEnrollTimeout, Category: cat, Kind: KindDuration, Default: "20m", Env: "APP_NODE_PROVISION_TIMEOUT", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(time.Minute, 24*time.Hour),
		Title:       "Cloud servers: enroll timeout",
		Description: "How long a provisioned server may take to enroll before it is failed and deleted, from 1m to 24h.",
	})
}
