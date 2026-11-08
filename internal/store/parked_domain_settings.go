package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// parkedSettingTables lists the per-domain settings tables a park keeps, in
// restore order (the auth gate row must exist before its access rules).
var parkedSettingTables = []string{
	"domain_waf",
	"domain_tls_cert",
	"domain_basic_auth",
	"domain_auth_gate",
	"gate_access_rules",
	"domain_maintenance",
	"domain_redirect",
	"domain_error_page",
	"domain_search_visibility",
	"domain_traffic_policies",
	"domain_limits",
}

var settingColumnName = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

type parkedSettingsSnapshot struct {
	Tables map[string][]map[string]any `json:"tables"`
}

// snapshotDomainSettingsTx reads every settings row for domain. Password and
// key material live in the secrets store keyed by domain, so only references
// and hashes already held in these tables are copied.
func snapshotDomainSettingsTx(ctx context.Context, tx *sql.Tx, domain string) (string, error) {
	snap := parkedSettingsSnapshot{Tables: map[string][]map[string]any{}}
	for _, table := range parkedSettingTables {
		rows, err := snapshotTableTx(ctx, tx, table, domain)
		if err != nil {
			return "", err
		}
		if len(rows) > 0 {
			snap.Tables[table] = rows
		}
	}
	if len(snap.Tables) == 0 {
		return "", nil
	}
	enc, err := json.Marshal(snap)
	if err != nil {
		return "", fmt.Errorf("store: encode settings snapshot for %q: %w", domain, err)
	}
	return string(enc), nil
}

// snapshotTableTx reads domain's rows of one settings table as column maps.
func snapshotTableTx(ctx context.Context, tx *sql.Tx, table, domain string) ([]map[string]any, error) {
	rows, err := tx.QueryContext(ctx, `SELECT * FROM `+table+` WHERE domain = ?`, domain) //nolint:gosec // table is from the fixed parkedSettingTables list
	if err != nil {
		return nil, fmt.Errorf("store: snapshot %s for %q: %w", table, domain, err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("store: snapshot %s columns: %w", table, err)
	}
	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("store: snapshot %s scan: %w", table, err)
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				row[c] = string(b)
				continue
			}
			row[c] = vals[i]
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: snapshot %s rows: %w", table, err)
	}
	return out, nil
}

// restoreDomainSettingsTx writes a snapshot back for a domain whose
// service_domains row already exists. It returns the table names restored.
func restoreDomainSettingsTx(ctx context.Context, tx *sql.Tx, domain, raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var snap parkedSettingsSnapshot
	if err := dec.Decode(&snap); err != nil {
		return nil, fmt.Errorf("store: decode settings snapshot for %q: %w", domain, err)
	}
	var restored []string
	for _, table := range parkedSettingTables {
		rows := snap.Tables[table]
		if len(rows) == 0 {
			continue
		}
		for _, row := range rows {
			cols := make([]string, 0, len(row))
			for c := range row {
				if !settingColumnName.MatchString(c) {
					return nil, fmt.Errorf("store: settings snapshot for %q has bad column %q", domain, c)
				}
				cols = append(cols, c)
			}
			sort.Strings(cols)
			args := make([]any, len(cols))
			for i, c := range cols {
				args[i] = snapshotArg(row[c])
				if c == "domain" {
					args[i] = domain
				}
			}
			q := `INSERT OR REPLACE INTO ` + table + ` (` + strings.Join(cols, ", ") + `) VALUES (?` + strings.Repeat(", ?", len(cols)-1) + `)` //nolint:gosec // table is fixed and columns match settingColumnName
			if _, err := tx.ExecContext(ctx, q, args...); err != nil {
				return nil, fmt.Errorf("store: restore %s for %q: %w", table, domain, err)
			}
		}
		restored = append(restored, table)
	}
	return restored, nil
}

func snapshotArg(v any) any {
	n, ok := v.(json.Number)
	if !ok {
		return v
	}
	if i, err := n.Int64(); err == nil {
		return i
	}
	if f, err := n.Float64(); err == nil {
		return f
	}
	return n.String()
}
