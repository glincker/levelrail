package dbviewer

import "fmt"

// External targets run the engine's client in a helper container that carries
// the connection in its environment (EXT_* and the standard client variables),
// so no password ever appears in an argument list.

func postgresExternalCmd(lim Limits, write bool) []string {
	ro := "on"
	if write {
		ro = "off"
	}
	opts := fmt.Sprintf("-c default_transaction_read_only=%s -c statement_timeout=%d -c lock_timeout=%d -c idle_in_transaction_session_timeout=%d -c standard_conforming_strings=on",
		ro, lim.Timeout.Milliseconds(), lim.Timeout.Milliseconds(), 2*lim.Timeout.Milliseconds())
	return []string{"sh", "-c", `PGOPTIONS="` + opts + `" exec psql -X -q --no-password --csv -v ON_ERROR_STOP=1`}
}

// MySQLSSLCase is the shell fragment that turns EXT_TLS into client flags,
// shared by the viewer and the connection probe.
func MySQLSSLCase(mariadb bool) string {
	const findCA = `CA=""; for f in /etc/ssl/certs/ca-certificates.crt /etc/pki/tls/certs/ca-bundle.crt /etc/ssl/certs/ca-bundle.crt; do if [ -f "$f" ]; then CA="$f"; break; fi; done
`
	if mariadb {
		return findCA + `case "$EXT_TLS" in disable) SSL="--skip-ssl";; require) SSL="--ssl --skip-ssl-verify-server-cert";; verify-ca|verify-full) SSL="--ssl --ssl-verify-server-cert --ssl-ca=$CA";; *) SSL="";; esac`
	}
	return findCA + `case "$EXT_TLS" in disable) SSL="--ssl-mode=DISABLED";; require) SSL="--ssl-mode=REQUIRED";; verify-ca) SSL="--ssl-mode=VERIFY_CA --ssl-ca=$CA";; verify-full) SSL="--ssl-mode=VERIFY_IDENTITY --ssl-ca=$CA";; *) SSL="";; esac`
}

func mysqlExternalCmd(mariadb bool) []string {
	return []string{"sh", "-c", MySQLSSLCase(mariadb) + `
BIN=$(command -v mysql || command -v mariadb)
DBOPT=""
if [ -n "$EXT_DB" ]; then DBOPT="--database=$EXT_DB"; fi
exec "$BIN" --protocol=tcp -h"$EXT_HOST" -P"$EXT_PORT" -u"$EXT_USER" $SSL $DBOPT --connect-timeout=10 --xml --skip-pager --default-character-set=utf8mb4`}
}
