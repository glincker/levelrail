package runtimesettings

import "time"

// SFTP backup target tunables. Env vars only seed the defaults; a saved value applies to the next connection.
const (
	KeyBackupSFTPAllowPrivate   = "backup.sftp_allow_private_networks" //nolint:gosec // setting key, not a credential
	KeyBackupSFTPConnectTimeout = "backup.sftp_connect_timeout"        //nolint:gosec // setting key, not a credential
	KeyBackupSFTPIdleTimeout    = "backup.sftp_idle_timeout"           //nolint:gosec // setting key, not a credential
	KeyBackupSFTPMaxEntries     = "backup.sftp_max_entries"            //nolint:gosec // setting key, not a credential
	KeyBackupSFTPMaxDepth       = "backup.sftp_max_depth"              //nolint:gosec // setting key, not a credential
	KeyBackupSFTPMaxConns       = "backup.sftp_max_connections"        //nolint:gosec // setting key, not a credential
	KeyBackupSFTPScanPerMinute  = "backup.sftp_scan_per_minute"        //nolint:gosec // setting key, not a credential
)

func init() {
	Register(Definition{
		Key: KeyBackupSFTPAllowPrivate, Category: CategorySecurity, Kind: KindBool, Default: "false",
		Env: "APP_BACKUP_SFTP_ALLOW_PRIVATE_NETWORKS", Apply: ApplyLive, Risk: RiskWeakensSecurity,
		Title:       "SFTP backup private networks",
		Description: "Lets SFTP backup targets reach private and loopback addresses. Metadata and link-local addresses stay refused. A change applies to the next connection.",
	})
	durDef := func(key, env, def, title, desc string, lo, hi time.Duration) {
		Register(Definition{
			Key: key, Category: CategoryDeploys, Kind: KindDuration, Default: def, Env: env, Apply: ApplyLive,
			CheckValue: durationRangeCheck(lo, hi), Title: title, Description: desc,
		})
	}
	durDef(KeyBackupSFTPConnectTimeout, "APP_BACKUP_SFTP_CONNECT_TIMEOUT", "15s", "SFTP connect timeout",
		"How long the TCP connect and SSH handshake to an SFTP server may take, from 1s to 5m.", time.Second, 5*time.Minute)
	durDef(KeyBackupSFTPIdleTimeout, "APP_BACKUP_SFTP_IDLE_TIMEOUT", "1m", "SFTP idle timeout",
		"Longest stall between SFTP reads, writes or calls before the connection is closed, from 1s to 1h.", time.Second, time.Hour)
	registerLimit(Definition{
		Key: KeyBackupSFTPMaxEntries, Category: CategoryDeploys, Default: "20000", Env: "APP_BACKUP_SFTP_MAX_ENTRIES", Apply: ApplyLive,
		Title:       "SFTP listing entry budget",
		Description: "Most directory entries one recursive SFTP listing may read. Between 100 and 1000000.",
	}, 100, 1000000)
	registerLimit(Definition{
		Key: KeyBackupSFTPMaxDepth, Category: CategoryDeploys, Default: "8", Env: "APP_BACKUP_SFTP_MAX_DEPTH", Apply: ApplyLive,
		Title:       "SFTP listing depth",
		Description: "Deepest folder level a recursive SFTP listing walks. Between 1 and 64.",
	}, 1, 64)
	registerLimit(Definition{
		Key: KeyBackupSFTPMaxConns, Category: CategoryDeploys, Default: "4", Env: "APP_BACKUP_SFTP_MAX_CONNECTIONS", Apply: ApplyLive,
		Title:       "SFTP concurrent connections",
		Description: "Most SFTP connections open at once across all targets; further operations wait. Between 1 and 32.",
	}, 1, 32)
	registerLimit(Definition{
		Key: KeyBackupSFTPScanPerMinute, Category: CategoryDeploys, Default: "10", Env: "APP_BACKUP_SFTP_SCAN_PER_MINUTE", Apply: ApplyLive,
		Title:       "SFTP host key scans per minute",
		Description: "Most SFTP host key scans one caller may start per minute. Between 1 and 600.",
	}, 1, 600)
}
