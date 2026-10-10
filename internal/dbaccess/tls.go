package dbaccess

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// TLS enforcement states as read from the server.
const (
	TLSRequired = "required"
	TLSOptional = "optional"
)

const hbaCatchAll = `[[:space:]]+all[[:space:]]+all[[:space:]]+all[[:space:]]`

// tlsStateScript prints required when every catch-all network rule is a
// hostssl rule (TCP without TLS matches nothing and is refused).
var tlsStateScript = `HBA="$PGDATA/pg_hba.conf"; ` +
	`if grep -Eq '^hostssl` + hbaCatchAll + `' "$HBA" && ! grep -Eq '^host` + hbaCatchAll + `' "$HBA"; then echo ` + TLSRequired + `; else echo ` + TLSOptional + `; fi`

func tlsSetScript(require bool) string {
	from, to := "host", "hostssl"
	if !require {
		from, to = "hostssl", "host"
	}
	return `set -e; HBA="$PGDATA/pg_hba.conf"; TMP="$(mktemp)"; ` +
		`sed -E 's/^` + from + `(` + hbaCatchAll + `)/` + to + `\1/' "$HBA" > "$TMP"; ` +
		`cat "$TMP" > "$HBA"; rm -f "$TMP"; ` +
		`exec psql -X -q --no-password -U "$POSTGRES_USER" "$POSTGRES_USER" -c 'SELECT pg_reload_conf()' >/dev/null`
}

// TLSState reads whether the server refuses non-TLS TCP connections.
func (p Postgres) TLSState(ctx context.Context) (string, error) {
	out, err := p.runShell(ctx, tlsStateScript)
	if err != nil {
		return "", fmt.Errorf("read tls enforcement: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// SetRequireTLS rewrites the catch-all network rules to hostssl (or back to
// host) and reloads the server. Local socket access, which the platform's own
// exec path uses, is untouched, so this cannot lock the control plane out.
func (p Postgres) SetRequireTLS(ctx context.Context, require bool) error {
	if _, err := p.runShell(ctx, tlsSetScript(require)); err != nil {
		return fmt.Errorf("set tls enforcement: %w", err)
	}
	return nil
}

func (p Postgres) runShell(ctx context.Context, script string) (string, error) {
	rc, err := p.Exec.ExecWithInput(ctx, p.ContainerID, []string{"sh", "-c", script}, strings.NewReader(""))
	if err != nil {
		return "", wrapExec(err)
	}
	defer func() { _ = rc.Close() }()
	out, err := io.ReadAll(rc)
	if err != nil {
		return "", wrapExec(err)
	}
	return string(out), nil
}
