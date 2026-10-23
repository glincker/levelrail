package demo

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Finding is one seed lint violation.
type Finding struct {
	Check  string
	Detail string
}

func (f Finding) String() string { return f.Check + ": " + f.Detail }

// rowQuerier is the slice of *sql.DB the linter reads through.
type rowQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// LintOptions tunes the linter. DomainSuffix is the one extra hostname suffix
// (the brand demo domain) allowed beside the reserved example names.
type LintOptions struct {
	DomainSuffix string
}

var (
	secretPatterns = map[string]*regexp.Regexp{
		"private key":    regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
		"aws key":        regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		"github token":   regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}\b`),
		"api key":        regexp.MustCompile(`\bsk-[A-Za-z0-9]{20,}\b`),
		"slack token":    regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`),
		"jwt":            regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`),
		"basic auth url": regexp.MustCompile(`://[^/\s:@]+:[^/\s@]+@`),
	}
	// Only real public suffixes count, so dotted identifiers such as
	// "app.deploy" are not mistaken for hostnames.
	hostPattern = regexp.MustCompile(`\b[a-z0-9][a-z0-9-]*(?:\.[a-z0-9-]+)*\.(?:com|net|org|io|dev|app|co|cloud|ai|xyz|me|us|uk|de|eu|sh|tech|info|biz|in|fr|nl|ru|cn|jp|site|online|page)\b`)
	ipPattern   = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)

	allowedHostSuffixes = []string{".example.com", ".example", ".invalid", ".internal", ".local", "docker.io", "example.com", "sslip.io", "nip.io", "traefik.me"}
	fileExtensions      = map[string]bool{
		"gz": true, "json": true, "sql": true, "yaml": true, "yml": true, "md": true, "txt": true, "go": true, "ts": true, "tsx": true,
		"js": true, "tgz": true, "zip": true, "dump": true, "log": true, "sum": true, "mod": true, "toml": true, "conf": true, "env": true,
		"py": true, "sh": true, "lock": true, "html": true, "css": true, "xml": true, "csv": true,
	}
	skipColumns = map[string]bool{"password_hash": true, "token_hash": true, "secret_hash": true}
	// Columns holding a deadline or a start of a future-looking window are
	// allowed to sit after the anchor.
	futureOK = map[string]bool{"expires_at": true, "cert_not_after": true, "not_after": true, "ends_at": true, "next_run_at": true, "scheduled_for": true, "valid_until": true}
)

// Lint checks a seeded data dir for dangling references, secret-looking
// values, non-example hostnames and timestamps after the seed anchor.
func Lint(ctx context.Context, st Stores, opts LintOptions) ([]Finding, error) {
	marker, err := st.DB.GetDemoMarker(ctx)
	if err != nil {
		return nil, fmt.Errorf("demo lint: read marker: %w", err)
	}
	l := &linter{ctx: ctx, anchor: marker.Anchor.UTC(), opts: opts}
	if err := l.references(st); err != nil {
		return nil, err
	}
	dbs := map[string]rowQuerier{"store": st.DB}
	if st.Telemetry != nil {
		dbs["telemetry"] = st.Telemetry
	}
	if st.Alerting != nil {
		dbs["alerting"] = st.Alerting
	}
	for name, q := range dbs {
		if err := l.scan(name, q); err != nil {
			return nil, err
		}
	}
	return l.findings, nil
}

type linter struct {
	ctx      context.Context
	anchor   time.Time
	opts     LintOptions
	findings []Finding
}

func (l *linter) add(check, format string, args ...any) {
	l.findings = append(l.findings, Finding{Check: check, Detail: fmt.Sprintf(format, args...)})
}

// orphans runs a query that returns the keys of rows whose parent is missing.
func (l *linter) orphans(q rowQuerier, label, query string) error {
	rows, err := q.QueryContext(l.ctx, query)
	if err != nil {
		return fmt.Errorf("demo lint: %s: %w", label, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return fmt.Errorf("demo lint: %s: %w", label, err)
		}
		l.add("missing-parent", "%s: %q", label, key)
	}
	return rows.Err()
}

func (l *linter) references(st Stores) error {
	checks := []struct{ label, query string }{
		{"deploy attempt without service", `SELECT id FROM deploy_attempts WHERE service_name NOT IN (SELECT name FROM desired_services)`},
		{"service with unknown node", `SELECT name FROM desired_services WHERE node_id <> '' AND node_id NOT IN (SELECT id FROM nodes)`},
		{"database with unknown node", `SELECT name FROM desired_databases WHERE node_id <> '' AND node_id NOT IN (SELECT id FROM nodes)`},
		{"service with unknown project", `SELECT name FROM desired_services WHERE project_id IS NOT NULL AND project_id NOT IN (SELECT id FROM projects)`},
		{"service with unknown environment", `SELECT name FROM desired_services WHERE environment_id IS NOT NULL AND environment_id NOT IN (SELECT id FROM environments)`},
		{"approval without service", `SELECT id FROM deploy_approvals WHERE service_name NOT IN (SELECT name FROM desired_services)`},
		{"budget without project", `SELECT id FROM usage_budgets WHERE scope = 'project' AND scope_id NOT IN (SELECT id FROM projects)`},
		{"team member without user", `SELECT user_id FROM team_members WHERE user_id NOT IN (SELECT id FROM users)`},
		{"status component without app", `SELECT id FROM status_components WHERE kind = 'app' AND target NOT IN (SELECT name FROM desired_services)`},
		{"incident update without incident", `SELECT id FROM status_incident_updates WHERE incident_id NOT IN (SELECT id FROM status_incidents)`},
		{"session audit actor without user", `SELECT id FROM audit_log WHERE actor_type = 'session' AND actor_id NOT IN (SELECT id FROM users)`},
	}
	for _, c := range checks {
		if err := l.orphans(st.DB, c.label, c.query); err != nil {
			return err
		}
	}
	if err := l.foreignKeys(st.DB); err != nil {
		return err
	}
	if st.Alerting != nil {
		for _, c := range []struct{ label, query string }{
			{"alert history without rule", `SELECT id FROM alert_history WHERE rule_id NOT IN (SELECT id FROM alert_rules)`},
			{"alert rule without channel", `SELECT id FROM alert_rules WHERE channel_id <> '' AND channel_id NOT IN (SELECT id FROM notification_channels)`},
		} {
			if err := l.orphans(st.Alerting, c.label, c.query); err != nil {
				return err
			}
		}
	}
	if st.Telemetry != nil {
		return l.telemetryResources(st)
	}
	return nil
}

func (l *linter) foreignKeys(db *store.DB) error {
	rows, err := db.QueryContext(l.ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("demo lint: foreign key check: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var table, parent string
		var rowid sql.NullInt64
		var fk sql.NullInt64
		if err := rows.Scan(&table, &rowid, &parent, &fk); err != nil {
			return fmt.Errorf("demo lint: foreign key check: %w", err)
		}
		l.add("missing-parent", "%s row %d references missing %s", table, rowid.Int64, parent)
	}
	return rows.Err()
}

// telemetryResources checks every service:/node: resource id in the
// telemetry store names a seeded service or node.
func (l *linter) telemetryResources(st Stores) error {
	known := map[string]bool{}
	for _, table := range []struct{ q, prefix string }{
		{`SELECT name FROM desired_services`, "service:"},
		{`SELECT name FROM desired_databases`, "database:"},
		{`SELECT id FROM nodes`, "node:"},
	} {
		rows, err := st.DB.QueryContext(l.ctx, table.q)
		if err != nil {
			return fmt.Errorf("demo lint: %w", err)
		}
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				_ = rows.Close()
				return fmt.Errorf("demo lint: %w", err)
			}
			known[table.prefix+n] = true
		}
		_ = rows.Close()
	}
	for _, t := range []string{"metric_samples", "log_entries"} {
		rows, err := st.Telemetry.QueryContext(l.ctx, `SELECT DISTINCT resource_id FROM `+t) //nolint:gosec // table name is a literal from the list above
		if err != nil {
			return fmt.Errorf("demo lint: %w", err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return fmt.Errorf("demo lint: %w", err)
			}
			if !known[id] {
				l.add("missing-parent", "%s resource %q has no seeded owner", t, id)
			}
		}
		_ = rows.Close()
	}
	return nil
}

// scan reads every user table as text and applies the content checks.
func (l *linter) scan(dbName string, q rowQuerier) error {
	tables, err := l.tables(q)
	if err != nil {
		return err
	}
	for _, table := range tables {
		cols, err := l.columns(q, table)
		if err != nil {
			return err
		}
		if err := l.scanTable(dbName, q, table, cols); err != nil {
			return err
		}
	}
	return nil
}

func (l *linter) tables(q rowQuerier) ([]string, error) {
	rows, err := q.QueryContext(l.ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE '%migrations%' ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("demo lint: list tables: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (l *linter) columns(q rowQuerier, table string) ([]string, error) {
	rows, err := q.QueryContext(l.ctx, fmt.Sprintf(`SELECT name FROM pragma_table_info(%q)`, table))
	if err != nil {
		return nil, fmt.Errorf("demo lint: columns of %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (l *linter) scanTable(dbName string, q rowQuerier, table string, cols []string) error {
	if len(cols) == 0 {
		return nil
	}
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = fmt.Sprintf(`CAST("%s" AS TEXT)`, c)
	}
	rows, err := q.QueryContext(l.ctx, fmt.Sprintf(`SELECT %s FROM "%s"`, strings.Join(quoted, ", "), table)) //nolint:gosec // identifiers come from sqlite_master
	if err != nil {
		return fmt.Errorf("demo lint: scan %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	vals := make([]sql.NullString, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return fmt.Errorf("demo lint: scan %s: %w", table, err)
		}
		for i, v := range vals {
			if v.Valid && v.String != "" && !skipColumns[cols[i]] {
				l.value(dbName+"."+table+"."+cols[i], cols[i], v.String)
			}
		}
	}
	return rows.Err()
}

func (l *linter) value(where, col, v string) {
	if len(v) > 20000 {
		v = v[:20000]
	}
	for name, re := range secretPatterns {
		if re.MatchString(v) {
			l.add("secret", "%s looks like a %s", where, name)
		}
	}
	for _, h := range hostPattern.FindAllString(v, -1) {
		if !l.hostAllowed(h) {
			l.add("domain", "%s mentions %q", where, h)
		}
	}
	for _, ip := range ipPattern.FindAllString(v, -1) {
		if !ipAllowed(ip) {
			l.add("ip", "%s mentions %q", where, ip)
		}
	}
	l.timestamp(where, col, v)
}

func (l *linter) hostAllowed(h string) bool {
	if idx := strings.LastIndex(h, "."); idx >= 0 && fileExtensions[h[idx+1:]] {
		return true
	}
	for _, s := range allowedHostSuffixes {
		if h == strings.TrimPrefix(s, ".") || strings.HasSuffix(h, s) {
			return true
		}
	}
	if l.opts.DomainSuffix != "" && (h == l.opts.DomainSuffix || strings.HasSuffix(h, "."+l.opts.DomainSuffix)) {
		return true
	}
	return false
}

func ipAllowed(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil {
		return true
	}
	for _, cidr := range []string{"10.0.0.0/8", "127.0.0.0/8", "192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24", "172.16.0.0/12", "0.0.0.0/32"} {
		if _, n, err := net.ParseCIDR(cidr); err == nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

// timestamp flags a timestamp-named column whose value is after the anchor.
func (l *linter) timestamp(where, col, v string) {
	if futureOK[col] {
		return
	}
	limit := l.anchor.Add(time.Minute)
	switch {
	case strings.HasSuffix(col, "_at") || col == "at" || col == "timestamp" || col == "created" || col == "day":
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
			if t, err := time.Parse(layout, v); err == nil {
				if t.After(limit) && layout != "2006-01-02" || layout == "2006-01-02" && t.After(l.anchor) {
					l.add("future-timestamp", "%s is %s, after the anchor %s", where, v, l.anchor.Format(time.RFC3339))
				}
				return
			}
		}
	case col == "ts":
		var n int64
		if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
			return
		}
		at := time.Unix(n, 0)
		if n > 1e15 {
			at = time.Unix(0, n)
		}
		if at.After(limit) {
			l.add("future-timestamp", "%s is %s, after the anchor %s", where, at.UTC().Format(time.RFC3339), l.anchor.Format(time.RFC3339))
		}
	}
}
