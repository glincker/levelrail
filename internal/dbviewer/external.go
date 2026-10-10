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

func mysqlExternalCmd(mariadb bool) []string {
	ssl := `case "$EXT_TLS" in disable) SSL="--ssl-mode=DISABLED";; require) SSL="--ssl-mode=REQUIRED";; *) SSL="";; esac`
	if mariadb {
		ssl = `case "$EXT_TLS" in disable) SSL="--skip-ssl";; require) SSL="--ssl --skip-ssl-verify-server-cert";; *) SSL="";; esac`
	}
	return []string{"sh", "-c", ssl + `
BIN=$(command -v mysql || command -v mariadb)
set --
if [ -n "$EXT_DB" ]; then set -- "$EXT_DB"; fi
exec "$BIN" --protocol=tcp -h"$EXT_HOST" -P"$EXT_PORT" -u"$EXT_USER" $SSL --connect-timeout=10 --xml --skip-pager --default-character-set=utf8mb4 "$@"`}
}
