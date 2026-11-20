package runtimesettings

import (
	"strconv"
	"time"
)

// WebDAV backup target tunables. Env vars only seed the defaults; a saved value applies to the next request.
const (
	KeyBackupWebDAVAllowPrivate    = "backup.webdav_allow_private_networks" //nolint:gosec // setting key, not a credential
	KeyBackupWebDAVHeaderTimeout   = "backup.webdav_header_timeout"         //nolint:gosec // setting key, not a credential
	KeyBackupWebDAVIdleTimeout     = "backup.webdav_idle_timeout"           //nolint:gosec // setting key, not a credential
	KeyBackupWebDAVRequestTimeout  = "backup.webdav_request_timeout"        //nolint:gosec // setting key, not a credential
	KeyBackupWebDAVMaxResponse     = "backup.webdav_max_response_bytes"     //nolint:gosec // setting key, not a credential
	KeyBackupWebDAVMaxEntries      = "backup.webdav_max_entries"            //nolint:gosec // setting key, not a credential
	KeyBackupWebDAVMaxDepth        = "backup.webdav_max_depth"              //nolint:gosec // setting key, not a credential
	backupWebDAVMaxResponseDefault = 8 << 20
)

func init() {
	Register(Definition{
		Key: KeyBackupWebDAVAllowPrivate, Category: CategorySecurity, Kind: KindBool, Default: "false",
		Env: "APP_BACKUP_WEBDAV_ALLOW_PRIVATE_NETWORKS", Apply: ApplyLive, Risk: RiskWeakensSecurity,
		Title:       "WebDAV backup private networks",
		Description: "Lets WebDAV backup targets use plain http and private or loopback addresses. Metadata and link-local addresses stay refused. A change applies to the next request.",
	})
	durDef := func(key, env, def, title, desc string, lo, hi time.Duration) {
		Register(Definition{
			Key: key, Category: CategoryDeploys, Kind: KindDuration, Default: def, Env: env, Apply: ApplyLive,
			CheckValue: durationRangeCheck(lo, hi), Title: title, Description: desc,
		})
	}
	durDef(KeyBackupWebDAVHeaderTimeout, "APP_BACKUP_WEBDAV_HEADER_TIMEOUT", "30s", "WebDAV header timeout",
		"How long a WebDAV server may take to start answering, from 1s to 10m.", time.Second, 10*time.Minute)
	durDef(KeyBackupWebDAVIdleTimeout, "APP_BACKUP_WEBDAV_IDLE_TIMEOUT", "1m", "WebDAV idle read timeout",
		"Longest stall between body reads before a WebDAV transfer is cancelled, from 1s to 1h.", time.Second, time.Hour)
	durDef(KeyBackupWebDAVRequestTimeout, "APP_BACKUP_WEBDAV_REQUEST_TIMEOUT", "2m", "WebDAV request timeout",
		"Overall cap for one small WebDAV call (not bulk transfers), from 1s to 1h.", time.Second, time.Hour)
	registerLimit(Definition{
		Key: KeyBackupWebDAVMaxResponse, Category: CategoryDeploys, Default: strconv.Itoa(backupWebDAVMaxResponseDefault), Env: "APP_BACKUP_WEBDAV_MAX_RESPONSE_BYTES", Apply: ApplyLive,
		Title:       "WebDAV listing response cap",
		Description: "Largest PROPFIND response body read from a WebDAV server, in bytes. Between 64 KiB and 256 MiB.",
	}, 64<<10, 256<<20)
	registerLimit(Definition{
		Key: KeyBackupWebDAVMaxEntries, Category: CategoryDeploys, Default: "20000", Env: "APP_BACKUP_WEBDAV_MAX_ENTRIES", Apply: ApplyLive,
		Title:       "WebDAV listing entry budget",
		Description: "Most requests plus entries one recursive WebDAV listing may spend. Between 100 and 1000000.",
	}, 100, 1000000)
	registerLimit(Definition{
		Key: KeyBackupWebDAVMaxDepth, Category: CategoryDeploys, Default: "8", Env: "APP_BACKUP_WEBDAV_MAX_DEPTH", Apply: ApplyLive,
		Title:       "WebDAV listing depth",
		Description: "Deepest folder level a recursive WebDAV listing walks. Between 1 and 64.",
	}, 1, 64)
}
