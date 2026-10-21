// Package forms captures HTML form posts for static sites: parsing, spam
// classification, storage, notification and export.
package forms

import (
	"strconv"
	"time"
)

// Environment variables that bound the public endpoint.
const (
	EnvMaxBodyBytes       = "FORMS_MAX_BODY_BYTES"
	EnvMaxFields          = "FORMS_MAX_FIELDS"
	EnvMaxFieldBytes      = "FORMS_MAX_FIELD_BYTES"
	EnvRatePerIPPerMinute = "FORMS_RATE_PER_IP_PER_MINUTE"
	EnvRatePerFormPerMin  = "FORMS_RATE_PER_FORM_PER_MINUTE"
	EnvRetentionDays      = "FORMS_RETENTION_DAYS"
	EnvMaxStoredPerForm   = "FORMS_MAX_STORED_PER_FORM"
	EnvMaxFormsPerApp     = "FORMS_MAX_PER_APP"
	EnvSpamMaxLinks       = "FORMS_SPAM_MAX_LINKS"
)

// Config holds the limits read from the environment; zero disables a limit.
type Config struct {
	MaxBodyBytes       int64
	MaxFields          int
	MaxFieldBytes      int
	RatePerIPPerMinute int
	RatePerFormPerMin  int
	Retention          time.Duration
	MaxStoredPerForm   int
	MaxFormsPerApp     int
	SpamMaxLinks       int
}

// LoadConfig reads the limits through lookup (os.LookupEnv in production).
func LoadConfig(lookup func(string) (string, bool)) Config {
	num := func(key string, def int64) int64 {
		raw, ok := lookup(key)
		if !ok {
			return def
		}
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			return def
		}
		return n
	}
	return Config{
		MaxBodyBytes:       num(EnvMaxBodyBytes, 1<<20),
		MaxFields:          int(num(EnvMaxFields, 100)),
		MaxFieldBytes:      int(num(EnvMaxFieldBytes, 16<<10)),
		RatePerIPPerMinute: int(num(EnvRatePerIPPerMinute, 10)),
		RatePerFormPerMin:  int(num(EnvRatePerFormPerMin, 120)),
		Retention:          time.Duration(num(EnvRetentionDays, 90)) * 24 * time.Hour,
		MaxStoredPerForm:   int(num(EnvMaxStoredPerForm, 5000)),
		MaxFormsPerApp:     int(num(EnvMaxFormsPerApp, 20)),
		SpamMaxLinks:       int(num(EnvSpamMaxLinks, 5)),
	}
}
