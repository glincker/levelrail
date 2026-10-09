package dbviewer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"
)

// timeoutMargin lets the database's own statement timeout fire before the
// client-side context does, so the caller sees a clean database error.
const timeoutMargin = 5 * time.Second

// Run executes a guard-approved statement. write must match the allowWrites
// value the statement was checked with.
func (t Target) Run(ctx context.Context, st Statement, write bool) (Result, error) {
	return t.run(ctx, st.SQL, st.Kind, write)
}

// RunTrusted executes SQL the control plane built itself (introspection,
// table pages). It still runs in a read-only transaction.
func (t Target) RunTrusted(ctx context.Context, sql string) (Result, error) {
	return t.run(ctx, sql, KindQuery, false)
}

// Explain returns the plan for a read statement as text lines (Postgres,
// MySQL, and MariaDB all accept an EXPLAIN prefix).
func (t Target) Explain(ctx context.Context, st Statement, analyze bool) (Result, error) {
	if st.Kind != KindQuery {
		return Result{}, fmt.Errorf("%w: only SELECT statements can be explained", ErrNotAllowed)
	}
	prefix := "EXPLAIN "
	switch {
	case t.Dialect == DialectPostgres && analyze:
		prefix = "EXPLAIN (ANALYZE, BUFFERS) "
	case analyze:
		prefix = "EXPLAIN ANALYZE "
	}
	return t.run(ctx, prefix+st.SQL, KindExplain, false)
}

func (t Target) run(ctx context.Context, sql, kind string, write bool) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, t.Limits.Timeout+timeoutMargin)
	defer cancel()

	var (
		cmd      []string
		script   string
		sentinel string
	)
	switch t.Dialect {
	case DialectPostgres:
		sentinel = nullSentinel()
		cmd = postgresCmd(t.Limits, write)
		script = postgresScript(sql, kind, write, t.Limits.MaxRows, sentinel)
	case DialectMySQL, DialectMariaDB:
		cmd = mysqlCmd()
		script = mysqlScript(sql, write, t.Limits.Timeout, t.Dialect == DialectMariaDB)
	default:
		return Result{}, fmt.Errorf("dbviewer: unsupported dialect %q", t.Dialect)
	}

	start := time.Now()
	rc, err := t.Exec.ExecWithInput(ctx, t.ContainerID, cmd, strings.NewReader(script))
	if err != nil {
		return Result{}, wrapExecError(ctx, err)
	}
	defer func() { _ = rc.Close() }()

	var res Result
	if t.Dialect == DialectPostgres {
		res, err = parseCSV(rc, sentinel, t.Limits)
	} else {
		res, err = parseXML(rc, t.Limits)
	}
	if err == nil && !res.Truncated {
		_, err = io.Copy(io.Discard, rc)
	}
	res.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		return Result{}, wrapExecError(ctx, err)
	}
	return res, nil
}

func nullSentinel() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "~null:" + hex.EncodeToString(b) + "~"
}

// postgresCmd execs psql over the container's local trust socket, the same
// way the dump path does. PGOPTIONS makes every transaction read-only by
// default even if a statement escaped its explicit BEGIN READ ONLY.
func postgresCmd(lim Limits, write bool) []string {
	ro := "on"
	if write {
		ro = "off"
	}
	opts := fmt.Sprintf("-c default_transaction_read_only=%s -c statement_timeout=%d -c lock_timeout=%d -c idle_in_transaction_session_timeout=%d -c standard_conforming_strings=on",
		ro, lim.Timeout.Milliseconds(), lim.Timeout.Milliseconds(), 2*lim.Timeout.Milliseconds())
	return []string{"sh", "-c", `PGOPTIONS="` + opts + `" exec psql -X -q --no-password --csv -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" "$POSTGRES_USER"`}
}

func postgresScript(sql, kind string, write bool, maxRows int, sentinel string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\\pset null '%s'\n", sentinel)
	switch {
	case write:
		b.WriteString("BEGIN;\n")
		b.WriteString(sql + "\n;\n")
		b.WriteString("COMMIT;\n")
	case kind == KindQuery:
		b.WriteString("BEGIN READ ONLY;\n")
		b.WriteString("DECLARE lr_cursor NO SCROLL CURSOR FOR\n" + sql + "\n;\n")
		fmt.Fprintf(&b, "FETCH FORWARD %d FROM lr_cursor;\n", maxRows+1)
		b.WriteString("ROLLBACK;\n")
	default:
		b.WriteString("BEGIN READ ONLY;\n")
		b.WriteString(sql + "\n;\n")
		b.WriteString("ROLLBACK;\n")
	}
	return b.String()
}

// mysqlCmd reads the root password from the container's own environment
// into MYSQL_PWD so it never appears in an argument list.
func mysqlCmd() []string {
	return []string{"sh", "-c", `BIN=$(command -v mysql || command -v mariadb); ` +
		`MYSQL_PWD="${MYSQL_ROOT_PASSWORD:-$MARIADB_ROOT_PASSWORD}" exec "$BIN" -uroot --xml --skip-pager ` +
		`--default-character-set=utf8mb4 "${MYSQL_DATABASE:-$MARIADB_DATABASE}"`}
}

func mysqlScript(sql string, write bool, timeout time.Duration, mariadb bool) string {
	var b strings.Builder
	if mariadb {
		fmt.Fprintf(&b, "SET SESSION max_statement_time=%.3f;\n", timeout.Seconds())
	} else {
		fmt.Fprintf(&b, "SET SESSION MAX_EXECUTION_TIME=%d;\n", timeout.Milliseconds())
	}
	if write {
		b.WriteString("START TRANSACTION;\n" + sql + "\n;\nCOMMIT;\n")
		return b.String()
	}
	b.WriteString("START TRANSACTION READ ONLY;\n" + sql + "\n;\nROLLBACK;\n")
	return b.String()
}
