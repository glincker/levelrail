package store

import (
	"context"
	"fmt"
	"sort"
)

// Space part ids: the groups of tables the Settings page reports and prunes together.
const (
	SpacePartDeploys = "deploys"
	SpacePartEvents  = "events"
	SpacePartAudit   = "audit"
)

// DBSpaceTable is one table or index and the bytes its pages occupy.
type DBSpaceTable struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// DBSpace is the page-level footprint of the control plane database.
type DBSpace struct {
	PageSize         int64          `json:"page_size"`
	PageCount        int64          `json:"page_count"`
	FreePages        int64          `json:"free_pages"`
	TotalBytes       int64          `json:"total_bytes"`
	ReclaimableBytes int64          `json:"reclaimable_bytes"`
	FreePercent      int            `json:"free_percent"`
	Tables           []DBSpaceTable `json:"tables"`
	Parts            []DBSpacePart  `json:"parts"`
}

// DBSpacePart is the page footprint of one group of tables with their indexes.
// CleanupTargets and Fix are filled by the API layer, which knows what can prune the part.
type DBSpacePart struct {
	ID             string   `json:"id"`
	Bytes          int64    `json:"bytes"`
	Tables         int      `json:"tables"`
	CleanupTargets []string `json:"cleanup_targets,omitempty"`
	Fix            string   `json:"fix,omitempty"`
}

// spacePartTables lists only the tables a cleanup target can prune, so a part's size and its prune action agree.
var spacePartTables = map[string]string{
	string(RetentionProbeAttempts): SpacePartDeploys, string(RetentionForgeStatus): SpacePartDeploys, string(RetentionDeployAttempts): SpacePartDeploys,
	string(RetentionNodeStatus): SpacePartEvents, string(RetentionNodeSecurity): SpacePartEvents,
	string(RetentionAppEvents): SpacePartEvents, string(RetentionAppSchedule): SpacePartEvents, string(RetentionCanaryOutcomes): SpacePartEvents,
	string(CleanupAuditLog): SpacePartAudit,
}

// SpacePartOf names the part a table belongs to, or "" for tables that belong to none.
func SpacePartOf(table string) string { return spacePartTables[table] }

// SpaceReport reads page counts and the largest tables. The per-table list is
// left empty when the SQLite build lacks the dbstat table.
func (db *DB) SpaceReport(ctx context.Context, topN int) (DBSpace, error) {
	var out DBSpace
	for pragma, dst := range map[string]*int64{
		"page_size":      &out.PageSize,
		"page_count":     &out.PageCount,
		"freelist_count": &out.FreePages,
	} {
		if err := db.DB.QueryRowContext(ctx, "PRAGMA "+pragma).Scan(dst); err != nil {
			return DBSpace{}, fmt.Errorf("store: space report %s: %w", pragma, err)
		}
	}
	out.TotalBytes = out.PageSize * out.PageCount
	out.ReclaimableBytes = out.PageSize * out.FreePages
	if out.PageCount > 0 {
		out.FreePercent = int(out.FreePages * 100 / out.PageCount)
	}
	out.Tables = []DBSpaceTable{}
	rows, err := db.DB.QueryContext(ctx, "SELECT name, SUM(pgsize) FROM dbstat GROUP BY name ORDER BY 2 DESC LIMIT ?", topN)
	if err != nil {
		return out, nil
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var t DBSpaceTable
		if rows.Scan(&t.Name, &t.Bytes) == nil {
			out.Tables = append(out.Tables, t)
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	out.Parts = db.spaceParts(ctx)
	return out, nil
}

// spaceParts sums table and index pages per part. Indexes count toward their table through sqlite_master.
func (db *DB) spaceParts(ctx context.Context) []DBSpacePart {
	parts := []DBSpacePart{}
	rows, err := db.DB.QueryContext(ctx, `SELECT COALESCE(m.tbl_name, d.name), SUM(d.pgsize) FROM dbstat d LEFT JOIN sqlite_master m ON m.name = d.name GROUP BY 1`)
	if err != nil {
		return parts
	}
	defer func() { _ = rows.Close() }()
	byID := map[string]*DBSpacePart{}
	for rows.Next() {
		var name string
		var n int64
		if rows.Scan(&name, &n) != nil {
			continue
		}
		id := SpacePartOf(name)
		if id == "" {
			continue
		}
		p := byID[id]
		if p == nil {
			p = &DBSpacePart{ID: id}
			byID[id] = p
		}
		p.Bytes += n
		p.Tables++
	}
	for _, p := range byID {
		parts = append(parts, *p)
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].ID < parts[j].ID })
	return parts
}

// CompactDatabase rewrites the file to give free pages back to the disk. It
// holds the writer for the duration, so callers run it only on request.
func (db *DB) CompactDatabase(ctx context.Context) error {
	if _, err := db.DB.ExecContext(ctx, "VACUUM"); err != nil {
		return fmt.Errorf("store: compact database: %w", err)
	}
	return nil
}
