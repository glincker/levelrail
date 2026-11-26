// Package mysqlssl holds the shell fragment that maps EXT_TLS to mysql client flags.
package mysqlssl

// Case is the shell fragment that turns EXT_TLS into client flags,
// shared by the viewer and the connection probe.
func Case(mariadb bool) string {
	const findCA = `CA=""; for f in /etc/ssl/certs/ca-certificates.crt /etc/pki/tls/certs/ca-bundle.crt /etc/ssl/certs/ca-bundle.crt; do if [ -f "$f" ]; then CA="$f"; break; fi; done
`
	if mariadb {
		return findCA + `case "$EXT_TLS" in disable) SSL="--skip-ssl";; require) SSL="--ssl --skip-ssl-verify-server-cert";; verify-ca|verify-full) SSL="--ssl --ssl-verify-server-cert --ssl-ca=$CA";; *) SSL="";; esac`
	}
	return findCA + `case "$EXT_TLS" in disable) SSL="--ssl-mode=DISABLED";; require) SSL="--ssl-mode=REQUIRED";; verify-ca) SSL="--ssl-mode=VERIFY_CA --ssl-ca=$CA";; verify-full) SSL="--ssl-mode=VERIFY_IDENTITY --ssl-ca=$CA";; *) SSL="";; esac`
}
