package secretscan

import (
	"strconv"
	"time"
)

// Environment variables that tune the scan caps.
const (
	envMaxFiles     = "APP_SECRET_SCAN_MAX_FILES"
	envMaxFileBytes = "APP_SECRET_SCAN_MAX_FILE_BYTES"
	envMaxTotal     = "APP_SECRET_SCAN_MAX_TOTAL_BYTES"
	envMaxFindings  = "APP_SECRET_SCAN_MAX_FINDINGS"
	envTimeout      = "APP_SECRET_SCAN_TIMEOUT"
)

// LimitsFromEnv reads the caps, keeping the default for a missing or invalid value.
func LimitsFromEnv(lookup func(string) (string, bool)) Limits {
	l := DefaultLimits()
	num := func(key string, dst *int64) {
		if v, ok := lookup(key); ok {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
				*dst = n
			}
		}
	}
	files, findings := int64(l.MaxFiles), int64(l.MaxFindings)
	num(envMaxFiles, &files)
	num(envMaxFindings, &findings)
	num(envMaxFileBytes, &l.MaxFileBytes)
	num(envMaxTotal, &l.MaxTotalBytes)
	l.MaxFiles, l.MaxFindings = int(files), int(findings)
	if v, ok := lookup(envTimeout); ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			l.Timeout = d
		}
	}
	return l
}
